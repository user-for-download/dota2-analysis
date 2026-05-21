package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/user-for-download/go-dota2-core/migrator"
	"github.com/user-for-download/go-dota2-core/schema"

	"github.com/user-for-download/go-dota2/internal/bootstrap"
	"github.com/user-for-download/go-dota2/internal/config"
)

func main() {
	log := bootstrap.NewLogger(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load("")
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdownTelemetry, err := bootstrap.InitTelemetry(ctx, "go-dota2-migrator", cfg.Telemetry.Endpoint, cfg.Telemetry.SampleRate)
	if err != nil {
		log.Error("init telemetry", "err", err)
	} else if shutdownTelemetry != nil {
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = shutdownTelemetry(shutdownCtx)
		}()
	}

	dsn := cfg.Migrator.DSN
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
	if err := migrator.Run(ctx, dsn, schema.Migrations, log); err != nil {
		log.Error("migration failed", "err", err)
		os.Exit(1)
	}
}

func getEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
