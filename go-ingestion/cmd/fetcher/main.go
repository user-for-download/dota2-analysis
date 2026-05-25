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

	shutdownTelemetry, err := bootstrap.InitTelemetry(ctx, "go-dota2-fetcher", cfg.Telemetry.Endpoint, cfg.Telemetry.SampleRate)
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

	rdb := core.Redis.Master()
	pool, err := bootstrap.ProxyPool(rdb, cfg.Proxy, log)
	must(log, "proxy pool", err)

	fetchDeps, err := bootstrap.BuildFetcher(ctx, cfg, log, core, pool)
	must(log, "build fetcher", err)
	defer fetchDeps.Close()

	log.Info("fetcher: starting")
	if err := fetchDeps.Worker.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("fetcher: stopped", "err", err)
	}
	log.Info("fetcher: stopped cleanly")
}

func must(log *slog.Logger, what string, err error) {
	if err != nil {
		log.Error(what, "err", err)
		os.Exit(1)
	}
}