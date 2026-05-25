package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/bootstrap"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/config"
)

func main() {
	log := bootstrap.NewLogger(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load("")
	must(log, "config", err)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdownTelemetry, err := bootstrap.InitTelemetry(ctx, "go-dota2-parser", cfg.Telemetry.Endpoint, cfg.Telemetry.SampleRate)
	if err != nil {
		log.Error("init telemetry", "err", err)
	} else if shutdownTelemetry != nil {
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = shutdownTelemetry(shutdownCtx)
		}()
	}

	core, err := bootstrap.Core(cfg, log)
	must(log, "core", err)
	defer core.Close()

	db, err := bootstrap.WaitForPostgres(ctx, cfg.Postgres, bootstrap.WaitConfig{
		Timeout:      0,
		PollInterval: 30 * time.Second,
	}, log)
	must(log, "postgres", err)
	defer db.Close()

	parserDeps, err := bootstrap.BuildParser(ctx, cfg, log, core, db)
	must(log, "build parser", err)

	// Start periodic partition maintenance to ensure future quarterly partitions exist.
	// Prevents data from falling into default partitions after the initial migration horizon.
	if parserDeps.PartitionAdmin != nil && cfg.Parser.PartitionMaintenanceInterval > 0 {
		go func() {
			ticker := time.NewTicker(cfg.Parser.PartitionMaintenanceInterval)
			defer ticker.Stop()
			// Run once immediately to catch up
			until := time.Now().AddDate(1, 0, 0)
			if err := parserDeps.PartitionAdmin.EnsurePartitions(ctx, until); err != nil {
				log.Warn("initial partition maintenance failed", "err", err)
			} else {
				log.Info("initial partition maintenance completed", "until", until)
			}
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					until := time.Now().AddDate(1, 0, 0)
					if err := parserDeps.PartitionAdmin.EnsurePartitions(ctx, until); err != nil {
						log.Warn("periodic partition maintenance failed", "err", err)
					} else {
						log.Info("partition maintenance completed", "until", until)
					}
				}
			}
		}()
	}

	log.Info("parser: starting")
	if err := parserDeps.Worker.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("parser: stopped", "err", err)
	}
	log.Info("parser: stopped cleanly")
}

func must(log *slog.Logger, what string, err error) {
	if err != nil {
		log.Error(what, "err", err)
		os.Exit(1)
	}
}