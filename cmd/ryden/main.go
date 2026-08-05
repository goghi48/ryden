package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/ryden-app/ryden/internal/attendance"
	"github.com/ryden-app/ryden/internal/auth"
	"github.com/ryden-app/ryden/internal/availability"
	"github.com/ryden-app/ryden/internal/calendar"
	"github.com/ryden-app/ryden/internal/config"
	"github.com/ryden-app/ryden/internal/decision"
	"github.com/ryden-app/ryden/internal/friendship"
	"github.com/ryden-app/ryden/internal/httpapi"
	"github.com/ryden-app/ryden/internal/live"
	"github.com/ryden-app/ryden/internal/media"
	"github.com/ryden-app/ryden/internal/meeting"
	"github.com/ryden-app/ryden/internal/meetinginvite"
	"github.com/ryden-app/ryden/internal/migrations"
	"github.com/ryden-app/ryden/internal/note"
	"github.com/ryden-app/ryden/internal/observability"
	"github.com/ryden-app/ryden/internal/poll"
	"github.com/ryden-app/ryden/internal/preparation"
	"golang.org/x/sync/errgroup"
)

func main() {
	if len(os.Args) == 3 && os.Args[1] == "healthcheck" {
		if err := healthcheck(os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(logger); err != nil {
		logger.Error("application stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	connectCtx, cancel := context.WithTimeout(rootCtx, 10*time.Second)
	defer cancel()
	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("parse database config: %w", err)
	}
	poolConfig.MaxConns = cfg.DatabaseMaxConns
	poolConfig.MinConns = cfg.DatabaseMinConns
	poolConfig.MaxConnLifetime = cfg.DatabaseMaxConnLifetime
	poolConfig.MaxConnLifetimeJitter = cfg.DatabaseMaxConnLifetime / 10
	poolConfig.MaxConnIdleTime = cfg.DatabaseMaxConnIdleTime
	poolConfig.HealthCheckPeriod = cfg.DatabaseHealthCheckPeriod
	poolConfig.ConnConfig.ConnectTimeout = cfg.DatabaseConnectTimeout
	if poolConfig.ConnConfig.RuntimeParams == nil {
		poolConfig.ConnConfig.RuntimeParams = make(map[string]string)
	}
	statementTimeoutMilliseconds := strconv.FormatInt(
		cfg.DatabaseStatementTimeout.Milliseconds(),
		10,
	)
	poolConfig.ConnConfig.RuntimeParams["statement_timeout"] = statementTimeoutMilliseconds
	poolConfig.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = statementTimeoutMilliseconds
	pool, err := pgxpool.NewWithConfig(connectCtx, poolConfig)
	if err != nil {
		return fmt.Errorf("create database pool: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(connectCtx); err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	if err := migrations.Apply(connectCtx, pool); err != nil {
		return fmt.Errorf("apply database migrations: %w", err)
	}

	tokenManager := auth.NewTokenManager(cfg.AccessTokenSecret, cfg.AccessTokenTTL)
	authService, err := auth.NewService(
		auth.NewPostgresRepository(pool),
		tokenManager,
		cfg.RefreshTokenTTL,
	)
	if err != nil {
		return fmt.Errorf("create authentication service: %w", err)
	}
	meetingService := meeting.NewService(meeting.NewPostgresRepository(pool))
	friendshipService := friendship.NewService(friendship.NewPostgresRepository(pool))
	meetingInviteService := meetinginvite.NewService(meetinginvite.NewPostgresRepository(pool))
	pollService := poll.NewService(poll.NewPostgresRepository(pool))
	availabilityService := availability.NewService(availability.NewPostgresRepository(pool))
	attendanceService := attendance.NewService(attendance.NewPostgresRepository(pool))
	noteService := note.NewService(note.NewPostgresRepository(pool))
	calendarService := calendar.NewService(meetingService)
	decisionService := decision.NewService(decision.NewPostgresRepository(pool))
	mediaService := media.NewService(media.NewPostgresRepository(pool))
	preparationService := preparation.NewService(preparation.NewPostgresRepository(pool))

	registry := prometheus.NewRegistry()
	registry.MustRegister(prometheus.NewGoCollector())
	registry.MustRegister(observability.NewDatabaseCollector(pool))
	metrics := observability.NewMetrics(registry)
	liveManager := live.NewManager(
		live.NewPostgresVersionSource(pool),
		metrics,
		logger,
		live.Options{
			PollInterval:        cfg.LivePollInterval,
			PollTimeout:         cfg.LivePollTimeout,
			MaxMeetings:         cfg.LiveMaxMeetings,
			MaxSubscribersPerID: cfg.LiveMaxSubscribersPerMeeting,
		},
	)
	handler := httpapi.NewServer(httpapi.Options{
		Auth:           authService,
		Friends:        friendshipService,
		MeetingInvites: meetingInviteService,
		Meetings:       meetingService,
		Polls:          pollService,
		Availability:   availabilityService,
		Attendance:     attendanceService,
		Notes:          noteService,
		Calendar:       calendarService,
		Decisions:      decisionService,
		Media:          mediaService,
		Preparation:    preparationService,
		Live:           liveManager,
		Tokens:         tokenManager,
		Pool:           pool,
		Metrics:        metrics,
		Logger:         logger,
		MetricsHandler: promhttp.HandlerFor(registry, promhttp.HandlerOpts{}),
		AllowedOrigin:  cfg.AllowedOrigin,
		CookieSecure:   cfg.CookieSecure,
	})
	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	group, groupCtx := errgroup.WithContext(rootCtx)
	group.Go(func() error {
		logger.Info("api started", "address", cfg.HTTPAddr)
		err := httpServer.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	})
	group.Go(func() error {
		return liveManager.Run(groupCtx)
	})
	group.Go(func() error {
		<-groupCtx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		return httpServer.Shutdown(shutdownCtx)
	})
	return group.Wait()
}

func healthcheck(url string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("health request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("health status: %s", response.Status)
	}
	return nil
}
