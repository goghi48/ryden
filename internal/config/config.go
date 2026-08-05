package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr                     string
	DatabaseURL                  string
	DatabaseMaxConns             int32
	DatabaseMinConns             int32
	DatabaseConnectTimeout       time.Duration
	DatabaseStatementTimeout     time.Duration
	DatabaseMaxConnLifetime      time.Duration
	DatabaseMaxConnIdleTime      time.Duration
	DatabaseHealthCheckPeriod    time.Duration
	AccessTokenSecret            string
	AllowedOrigin                string
	CookieSecure                 bool
	AccessTokenTTL               time.Duration
	RefreshTokenTTL              time.Duration
	LivePollInterval             time.Duration
	LivePollTimeout              time.Duration
	LiveMaxMeetings              int
	LiveMaxSubscribersPerMeeting int
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:          envOrDefault("RYDEN_HTTP_ADDR", ":8080"),
		DatabaseURL:       strings.TrimSpace(os.Getenv("RYDEN_DATABASE_URL")),
		AccessTokenSecret: os.Getenv("RYDEN_ACCESS_TOKEN_SECRET"),
		AllowedOrigin:     envOrDefault("RYDEN_ALLOWED_ORIGIN", "http://localhost:5173"),
		AccessTokenTTL:    15 * time.Minute,
		RefreshTokenTTL:   30 * 24 * time.Hour,
	}

	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("RYDEN_DATABASE_URL is required")
	}
	if len(cfg.AccessTokenSecret) < 32 {
		return Config{}, errors.New("RYDEN_ACCESS_TOKEN_SECRET must contain at least 32 characters")
	}

	secure, err := strconv.ParseBool(envOrDefault("RYDEN_COOKIE_SECURE", "true"))
	if err != nil {
		return Config{}, fmt.Errorf("parse RYDEN_COOKIE_SECURE: %w", err)
	}
	cfg.CookieSecure = secure

	if cfg.DatabaseMaxConns, err = envInt32("RYDEN_DATABASE_MAX_CONNS", 10, 1, 1000); err != nil {
		return Config{}, err
	}
	if cfg.DatabaseMinConns, err = envInt32("RYDEN_DATABASE_MIN_CONNS", 1, 0, 1000); err != nil {
		return Config{}, err
	}
	if cfg.DatabaseMinConns > cfg.DatabaseMaxConns {
		return Config{}, errors.New("RYDEN_DATABASE_MIN_CONNS must not exceed RYDEN_DATABASE_MAX_CONNS")
	}
	if cfg.DatabaseConnectTimeout, err = envDuration(
		"RYDEN_DATABASE_CONNECT_TIMEOUT", 5*time.Second, time.Second, time.Minute,
	); err != nil {
		return Config{}, err
	}
	if cfg.DatabaseStatementTimeout, err = envDuration(
		"RYDEN_DATABASE_STATEMENT_TIMEOUT", 8*time.Second, time.Second, time.Minute,
	); err != nil {
		return Config{}, err
	}
	if cfg.DatabaseMaxConnLifetime, err = envDuration(
		"RYDEN_DATABASE_MAX_CONN_LIFETIME", 30*time.Minute, time.Minute, 24*time.Hour,
	); err != nil {
		return Config{}, err
	}
	if cfg.DatabaseMaxConnIdleTime, err = envDuration(
		"RYDEN_DATABASE_MAX_CONN_IDLE_TIME", 5*time.Minute, time.Minute, 24*time.Hour,
	); err != nil {
		return Config{}, err
	}
	if cfg.DatabaseHealthCheckPeriod, err = envDuration(
		"RYDEN_DATABASE_HEALTH_CHECK_PERIOD", time.Minute, 10*time.Second, 10*time.Minute,
	); err != nil {
		return Config{}, err
	}
	if cfg.LivePollInterval, err = envDuration(
		"RYDEN_LIVE_POLL_INTERVAL", 2*time.Second, 250*time.Millisecond, time.Minute,
	); err != nil {
		return Config{}, err
	}
	if cfg.LivePollTimeout, err = envDuration(
		"RYDEN_LIVE_POLL_TIMEOUT", 1500*time.Millisecond, 100*time.Millisecond, time.Minute,
	); err != nil {
		return Config{}, err
	}
	if cfg.LivePollTimeout > cfg.LivePollInterval {
		return Config{}, errors.New("RYDEN_LIVE_POLL_TIMEOUT must not exceed RYDEN_LIVE_POLL_INTERVAL")
	}
	if cfg.LiveMaxMeetings, err = envInt("RYDEN_LIVE_MAX_MEETINGS", 1000, 1, 100000); err != nil {
		return Config{}, err
	}
	if cfg.LiveMaxSubscribersPerMeeting, err = envInt(
		"RYDEN_LIVE_MAX_SUBSCRIBERS_PER_MEETING", 100, 1, 10000,
	); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func envInt32(name string, fallback, minimum, maximum int32) (int32, error) {
	value, err := envInt64(name, int64(fallback), int64(minimum), int64(maximum))
	return int32(value), err
}

func envInt(name string, fallback, minimum, maximum int) (int, error) {
	value, err := envInt64(name, int64(fallback), int64(minimum), int64(maximum))
	return int(value), err
}

func envInt64(name string, fallback, minimum, maximum int64) (int64, error) {
	raw := envOrDefault(name, strconv.FormatInt(fallback, 10))
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", name, err)
	}
	if value < minimum || value > maximum {
		return 0, fmt.Errorf("%s must be between %d and %d", name, minimum, maximum)
	}
	return value, nil
}

func envDuration(
	name string,
	fallback, minimum, maximum time.Duration,
) (time.Duration, error) {
	raw := envOrDefault(name, fallback.String())
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", name, err)
	}
	if value < minimum || value > maximum {
		return 0, fmt.Errorf("%s must be between %s and %s", name, minimum, maximum)
	}
	return value, nil
}
