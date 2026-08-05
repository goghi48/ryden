package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadUsesEconomicalDatabaseAndLiveDefaults(t *testing.T) {
	setRequiredEnvironment(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.DatabaseMaxConns != 10 || cfg.DatabaseMinConns != 1 {
		t.Fatalf(
			"database pool = %d..%d, want 1..10",
			cfg.DatabaseMinConns,
			cfg.DatabaseMaxConns,
		)
	}
	if cfg.DatabaseMaxConnIdleTime != 5*time.Minute {
		t.Fatalf("DatabaseMaxConnIdleTime = %s, want 5m", cfg.DatabaseMaxConnIdleTime)
	}
	if cfg.DatabaseStatementTimeout != 8*time.Second {
		t.Fatalf("DatabaseStatementTimeout = %s, want 8s", cfg.DatabaseStatementTimeout)
	}
	if cfg.LivePollInterval != 2*time.Second || cfg.LivePollTimeout != 1500*time.Millisecond {
		t.Fatalf(
			"live polling = %s/%s, want 2s/1.5s",
			cfg.LivePollInterval,
			cfg.LivePollTimeout,
		)
	}
}

func TestLoadAcceptsBoundedCapacityOverrides(t *testing.T) {
	setRequiredEnvironment(t)
	t.Setenv("RYDEN_DATABASE_MAX_CONNS", "32")
	t.Setenv("RYDEN_DATABASE_MIN_CONNS", "4")
	t.Setenv("RYDEN_DATABASE_CONNECT_TIMEOUT", "7s")
	t.Setenv("RYDEN_DATABASE_STATEMENT_TIMEOUT", "6s")
	t.Setenv("RYDEN_DATABASE_MAX_CONN_LIFETIME", "45m")
	t.Setenv("RYDEN_DATABASE_MAX_CONN_IDLE_TIME", "3m")
	t.Setenv("RYDEN_DATABASE_HEALTH_CHECK_PERIOD", "30s")
	t.Setenv("RYDEN_LIVE_POLL_INTERVAL", "3s")
	t.Setenv("RYDEN_LIVE_POLL_TIMEOUT", "2s")
	t.Setenv("RYDEN_LIVE_MAX_MEETINGS", "2500")
	t.Setenv("RYDEN_LIVE_MAX_SUBSCRIBERS_PER_MEETING", "250")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.DatabaseMaxConns != 32 || cfg.DatabaseMinConns != 4 {
		t.Fatalf("database pool = %d..%d, want 4..32", cfg.DatabaseMinConns, cfg.DatabaseMaxConns)
	}
	if cfg.DatabaseConnectTimeout != 7*time.Second ||
		cfg.DatabaseStatementTimeout != 6*time.Second ||
		cfg.DatabaseMaxConnLifetime != 45*time.Minute ||
		cfg.DatabaseMaxConnIdleTime != 3*time.Minute ||
		cfg.DatabaseHealthCheckPeriod != 30*time.Second {
		t.Fatalf("database durations = %#v", cfg)
	}
	if cfg.LivePollInterval != 3*time.Second ||
		cfg.LivePollTimeout != 2*time.Second ||
		cfg.LiveMaxMeetings != 2500 ||
		cfg.LiveMaxSubscribersPerMeeting != 250 {
		t.Fatalf("live options = %#v", cfg)
	}
}

func TestLoadRejectsDatabaseMinimumAboveMaximum(t *testing.T) {
	setRequiredEnvironment(t)
	t.Setenv("RYDEN_DATABASE_MAX_CONNS", "4")
	t.Setenv("RYDEN_DATABASE_MIN_CONNS", "5")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "must not exceed") {
		t.Fatalf("Load() error = %v, want minimum/maximum validation", err)
	}
}

func TestLoadRejectsLiveTimeoutAbovePollInterval(t *testing.T) {
	setRequiredEnvironment(t)
	t.Setenv("RYDEN_LIVE_POLL_INTERVAL", "1s")
	t.Setenv("RYDEN_LIVE_POLL_TIMEOUT", "2s")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "must not exceed") {
		t.Fatalf("Load() error = %v, want live timeout validation", err)
	}
}

func setRequiredEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("RYDEN_DATABASE_URL", "postgres://ryden:secret@localhost:5432/ryden")
	t.Setenv("RYDEN_ACCESS_TOKEN_SECRET", "test-access-token-secret-that-is-long-enough")
	t.Setenv("RYDEN_COOKIE_SECURE", "true")
	for _, name := range []string{
		"RYDEN_DATABASE_MAX_CONNS",
		"RYDEN_DATABASE_MIN_CONNS",
		"RYDEN_DATABASE_CONNECT_TIMEOUT",
		"RYDEN_DATABASE_STATEMENT_TIMEOUT",
		"RYDEN_DATABASE_MAX_CONN_LIFETIME",
		"RYDEN_DATABASE_MAX_CONN_IDLE_TIME",
		"RYDEN_DATABASE_HEALTH_CHECK_PERIOD",
		"RYDEN_LIVE_POLL_INTERVAL",
		"RYDEN_LIVE_POLL_TIMEOUT",
		"RYDEN_LIVE_MAX_MEETINGS",
		"RYDEN_LIVE_MAX_SUBSCRIBERS_PER_MEETING",
	} {
		t.Setenv(name, "")
	}
}
