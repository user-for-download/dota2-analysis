package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/user-for-download/go-dota2/internal/bootstrap"
	"github.com/user-for-download/go-dota2/internal/config"
)

func main() {
	log := bootstrap.NewLogger(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load("")
	must(log, "config", err)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdownTelemetry, err := bootstrap.InitTelemetry(ctx, "go-dota2-enricher", cfg.Telemetry.Endpoint, cfg.Telemetry.SampleRate)
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

	if !cfg.Enrich.AllowDirect {
		waitCfg := bootstrap.WaitConfig{
			MinSize:      cfg.Proxy.MinPoolSize,
			Timeout:     cfg.Enrich.WaitTimeout,
			PollInterval: 2 * time.Second,
		}
		must(log, "proxies", bootstrap.WaitForProxies(ctx, pool, waitCfg, log))
	}

	db, err := bootstrap.WaitForPostgres(ctx, cfg.Postgres, bootstrap.WaitConfig{
		Timeout:      0,
		PollInterval: 30 * time.Second,
	}, log)
	must(log, "postgres", err)
	defer db.Close()

	enrichDeps, err := bootstrap.BuildEnricher(ctx, cfg, log, pool, rdb, db)
	must(log, "build enricher", err)
	defer enrichDeps.Close()

	if enrichDeps.HasLocal {
		log.Info("enricher: running local bootstrap from " + cfg.Enrich.LocalBootstrapDir)
		if err := enrichDeps.LocalRunner.Run(ctx); err != nil {
			log.Warn("enricher: local bootstrap failed, falling back to remote", "err", err)
		} else {
			log.Info("enricher: local bootstrap completed successfully")
		}
	}

	run := func() {
		log.Info("enricher: running enrichment cycle")
		if err := enrichDeps.MainRunner.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Error("enricher: run failed", "err", err)
		}
	}

	if cfg.Enrich.RunAtStart {
		run()
	}

	if cfg.Enrich.Interval <= 0 {
		log.Info("enricher: interval <= 0, exiting")
		return
	}

	ticker := time.NewTicker(cfg.Enrich.Interval)
	defer ticker.Stop()
	log.Info("enricher: listening on interval schedule", "interval", cfg.Enrich.Interval)

	for {
		select {
		case <-ctx.Done():
			log.Info("enricher: stopped cleanly")
			return
		case <-ticker.C:
			run()
		}
	}
}

func must(log *slog.Logger, what string, err error) {
	if err != nil {
		log.Error(what, "err", err)
		os.Exit(1)
	}
}