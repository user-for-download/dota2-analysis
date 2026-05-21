package config

import (
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Postgres  PostgresConfig
	Analytics AnalyticsConfig
	API       APIConfig
	Telemetry TelemetryConfig
	Migrator  MigratorConfig
}

type PostgresConfig struct {
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

type AnalyticsConfig struct {
	CurrentPatchID     int32
	ModelDir           string
	ValueModelDir      string
	ScorerKind         string
	FeaturizerInterval time.Duration
}

type APIConfig struct {
	Token    string
	BindAddr string
}

type TelemetryConfig struct {
	Endpoint   string
	SampleRate float64
}

type MigratorConfig struct {
	DSN           string
	MigrationsDir string
}

func Load(path string) (*Config, error) {
	if path != "" {
		_ = godotenv.Load(path)
	}

	return &Config{
		Postgres: PostgresConfig{
			DSN:             getStr("POSTGRES_DSN", ""),
			MaxOpenConns:    getInt("POSTGRES_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    getInt("POSTGRES_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: getDur("POSTGRES_CONN_MAX_LIFETIME", 30*time.Minute),
			ConnMaxIdleTime: getDur("POSTGRES_CONN_MAX_IDLE_TIME", 10*time.Minute),
		},
		Analytics: AnalyticsConfig{
			CurrentPatchID:     int32(getInt("ANALYTICS_PATCH_ID", 0)),
			ModelDir:           getStr("ANALYTICS_MODEL_DIR", "./deploy/models/imitation/current"),
			ValueModelDir:      getStr("ANALYTICS_VALUE_MODEL_DIR", "./deploy/models/value/current"),
			ScorerKind:         getStr("ANALYTICS_SCORER_KIND", "linear"),
			FeaturizerInterval: getDur("ANALYTICS_FEATURIZER_INTERVAL", 24*time.Hour),
		},
		API: APIConfig{
			Token:    getStr("API_TOKEN", ""),
			BindAddr: getStr("API_BIND", "127.0.0.1:8080"),
		},
		Telemetry: TelemetryConfig{
			Endpoint:   getStr("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
			SampleRate: getFloat("OTEL_SAMPLE_RATE", 1.0),
		},
		Migrator: MigratorConfig{
			DSN:           getStr("MIGRATOR_DSN", ""),
			MigrationsDir: getStr("MIGRATOR_MIGRATIONS_DIR", "/migrations"),
		},
	}, nil
}

func getStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			return n
		}
	}
	return def
}

func getDur(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if n, err := time.ParseDuration(v); err == nil {
			return n
		}
	}
	return def
}
