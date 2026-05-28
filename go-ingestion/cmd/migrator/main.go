package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/user-for-download/dota2-analysis/go-core/migrator"
	"github.com/user-for-download/dota2-analysis/go-core/schema"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/bootstrap"
)

func main() {
	log := bootstrap.NewLoggerFromEnv()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdownTelemetry, err := bootstrap.InitTelemetry(ctx, "go-dota2-migrator",
		getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
		getEnvFloat("OTEL_SAMPLE_RATE", 1.0))
	if err != nil {
		log.Error("init telemetry", "err", err)
	} else if shutdownTelemetry != nil {
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = shutdownTelemetry(shutdownCtx)
		}()
	}

	dsn := getEnv("MIGRATOR_DSN", "")
	if dsn == "" {
		dsn = fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
			getEnv("POSTGRES_HOST", "postgres"),
			getEnv("POSTGRES_PORT", "5432"),
			getEnv("POSTGRES_USER", "dota2"),
			getEnv("POSTGRES_PASSWORD", "dota2"),
			getEnv("POSTGRES_DB", "dota2"),
			getEnv("POSTGRES_SSLMODE", "disable"),
		)
	}
	if dsn == "" {
		log.Error("migrator: DSN required")
		os.Exit(1)
	}

	log.Info("running migrations from embedded schema")
	migFS, err := fs.Sub(schema.Migrations, "migrations")
	if err != nil {
		log.Error("sub migrations", "err", err)
		os.Exit(1)
	}
	if err := migrator.Run(ctx, dsn, migFS, log); err != nil {
		log.Error("migration failed", "err", err)
		os.Exit(1)
	}
}

func getEnvFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			return n
		}
	}
	return def
}

func getEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
