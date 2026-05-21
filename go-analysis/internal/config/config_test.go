package config

import (
	"os"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	// Clear any env vars that might interfere
	clearEnv()

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Postgres defaults
	if cfg.Postgres.MaxOpenConns != 25 {
		t.Errorf("Postgres.MaxOpenConns = %d, want 25", cfg.Postgres.MaxOpenConns)
	}
	if cfg.Postgres.MaxIdleConns != 5 {
		t.Errorf("Postgres.MaxIdleConns = %d, want 5", cfg.Postgres.MaxIdleConns)
	}
	if cfg.Postgres.ConnMaxLifetime != 30*time.Minute {
		t.Errorf("Postgres.ConnMaxLifetime = %v, want 30m", cfg.Postgres.ConnMaxLifetime)
	}
	if cfg.Postgres.ConnMaxIdleTime != 10*time.Minute {
		t.Errorf("Postgres.ConnMaxIdleTime = %v, want 10m", cfg.Postgres.ConnMaxIdleTime)
	}

	// Analytics defaults
	if cfg.Analytics.CurrentPatchID != 0 {
		t.Errorf("Analytics.CurrentPatchID = %d, want 0", cfg.Analytics.CurrentPatchID)
	}
	if cfg.Analytics.ModelDir != "./deploy/models/imitation/current" {
		t.Errorf("Analytics.ModelDir = %q, want %q", cfg.Analytics.ModelDir, "./deploy/models/imitation/current")
	}
	if cfg.Analytics.ValueModelDir != "./deploy/models/value/current" {
		t.Errorf("Analytics.ValueModelDir = %q, want %q", cfg.Analytics.ValueModelDir, "./deploy/models/value/current")
	}
	if cfg.Analytics.ScorerKind != "linear" {
		t.Errorf("Analytics.ScorerKind = %q, want %q", cfg.Analytics.ScorerKind, "linear")
	}
	if cfg.Analytics.FeaturizerInterval != 24*time.Hour {
		t.Errorf("Analytics.FeaturizerInterval = %v, want 24h", cfg.Analytics.FeaturizerInterval)
	}

	// API defaults
	if cfg.API.Token != "" {
		t.Errorf("API.Token = %q, want empty", cfg.API.Token)
	}
	if cfg.API.BindAddr != "127.0.0.1:8080" {
		t.Errorf("API.BindAddr = %q, want %q", cfg.API.BindAddr, "127.0.0.1:8080")
	}

	// Telemetry defaults
	if cfg.Telemetry.SampleRate != 1.0 {
		t.Errorf("Telemetry.SampleRate = %f, want 1.0", cfg.Telemetry.SampleRate)
	}

	// Migrator defaults
	if cfg.Migrator.MigrationsDir != "/migrations" {
		t.Errorf("Migrator.MigrationsDir = %q, want %q", cfg.Migrator.MigrationsDir, "/migrations")
	}
}

func TestLoadEnvOverrides(t *testing.T) {
	clearEnv()

	os.Setenv("ANALYTICS_PATCH_ID", "7")
	os.Setenv("ANALYTICS_SCORER_KIND", "lgbm")
	os.Setenv("API_BIND", ":9090")
	os.Setenv("POSTGRES_MAX_OPEN_CONNS", "50")
	os.Setenv("ANALYTICS_FEATURIZER_INTERVAL", "12h")
	os.Setenv("OTEL_SAMPLE_RATE", "0.5")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Analytics.CurrentPatchID != 7 {
		t.Errorf("Analytics.CurrentPatchID = %d, want 7", cfg.Analytics.CurrentPatchID)
	}
	if cfg.Analytics.ScorerKind != "lgbm" {
		t.Errorf("Analytics.ScorerKind = %q, want %q", cfg.Analytics.ScorerKind, "lgbm")
	}
	if cfg.API.BindAddr != ":9090" {
		t.Errorf("API.BindAddr = %q, want %q", cfg.API.BindAddr, ":9090")
	}
	if cfg.Postgres.MaxOpenConns != 50 {
		t.Errorf("Postgres.MaxOpenConns = %d, want 50", cfg.Postgres.MaxOpenConns)
	}
	if cfg.Analytics.FeaturizerInterval != 12*time.Hour {
		t.Errorf("Analytics.FeaturizerInterval = %v, want 12h", cfg.Analytics.FeaturizerInterval)
	}
	if cfg.Telemetry.SampleRate != 0.5 {
		t.Errorf("Telemetry.SampleRate = %f, want 0.5", cfg.Telemetry.SampleRate)
	}
}

func clearEnv() {
	os.Unsetenv("POSTGRES_DSN")
	os.Unsetenv("POSTGRES_MAX_OPEN_CONNS")
	os.Unsetenv("POSTGRES_MAX_IDLE_CONNS")
	os.Unsetenv("POSTGRES_CONN_MAX_LIFETIME")
	os.Unsetenv("POSTGRES_CONN_MAX_IDLE_TIME")
	os.Unsetenv("ANALYTICS_PATCH_ID")
	os.Unsetenv("ANALYTICS_MODEL_DIR")
	os.Unsetenv("ANALYTICS_VALUE_MODEL_DIR")
	os.Unsetenv("ANALYTICS_SCORER_KIND")
	os.Unsetenv("ANALYTICS_FEATURIZER_INTERVAL")
	os.Unsetenv("API_TOKEN")
	os.Unsetenv("API_BIND")
	os.Unsetenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	os.Unsetenv("OTEL_SAMPLE_RATE")
	os.Unsetenv("MIGRATOR_DSN")
	os.Unsetenv("MIGRATOR_MIGRATIONS_DIR")
}
