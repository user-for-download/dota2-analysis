package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/user-for-download/go-dota2/internal/bootstrap"
	"github.com/user-for-download/go-dota2/internal/config"
	"github.com/user-for-download/go-dota2/internal/proxy/httpdo"
	"github.com/user-for-download/go-dota2/internal/worker/discovery"
)

func main() {
	log := bootstrap.NewLogger(slog.NewJSONHandler(os.Stdout, nil))

	fs := flag.NewFlagSet("discoverer", flag.ExitOnError)
	fileKey := fs.String("file", "", "run only this query key (filename without .sql); one-shot (matches only)")
	if err := fs.Parse(os.Args[1:]); err != nil {
		log.Error("parse flags", "err", err)
		os.Exit(2)
	}

	cfg, err := config.Load("")
	must(log, "config", err)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdownTelemetry, err := bootstrap.InitTelemetry(ctx, "go-dota2-discoverer", cfg.Telemetry.Endpoint, cfg.Telemetry.SampleRate)
	if err != nil {
		log.Error("init telemetry", "err", err)
	} else if shutdownTelemetry != nil {
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = shutdownTelemetry(shutdownCtx)
		}()
	}

	deps, err := bootstrap.Core(cfg, log)
	must(log, "core", err)
	defer deps.Close()
	rdb := deps.Redis.Master()

	pool, err := bootstrap.ProxyPool(rdb, cfg.Proxy, log)
	must(log, "proxy pool", err)

	if !cfg.Discovery.AllowDirect {
		waitCfg := bootstrap.WaitConfig{
			MinSize:      cfg.Discovery.MinProxyPoolSize,
			Timeout:     cfg.Discovery.WaitTimeout,
			PollInterval: 2 * time.Second,
		}
		if err := bootstrap.WaitForProxies(ctx, pool, waitCfg, log); err != nil {
			log.Error("discoverer: proxies not ready", "err", err)
			os.Exit(1)
		}
	}

	fetchQ, err := bootstrap.FetchQueue(rdb, cfg.Queue, log)
	must(log, "fetch queue", err)

	dedupSeen, err := bootstrap.DedupSeen(rdb, cfg.Dedup)
	if err != nil {
		log.Warn("dedup: init failed, proceeding without dedup", "err", err)
	}

	queries, err := discovery.LoadQueries(cfg.Discovery.QueriesDir)
	must(log, "load queries", err)
	log.Info("queries loaded", "dir", cfg.Discovery.QueriesDir, "count", len(queries))

	doer, err := httpdo.New(httpdo.Config{
		Pool:        pool,
		Hold:        cfg.Proxy.Hold,
		Timeout:     cfg.Discovery.HTTPTimeout,
		MaxRetries:  cfg.Discovery.MaxRetries,
		Backoff:     cfg.Discovery.RetryBackoff,
		AllowDirect: cfg.Discovery.AllowDirect,
		Logger:      log,
	})
	must(log, "httpdo", err)
	defer doer.Close()

	discDeps, err := bootstrap.BuildDiscoverer(ctx, cfg, log, fetchQ, doer, dedupSeen, deps.Metrics, queries, *fileKey)
	if err != nil {
		log.Error("build discoverer", "err", err)
		os.Exit(1)
	}
	if discDeps.Pool != nil {
		defer discDeps.Pool.Close()
	}

	if *fileKey != "" {
		if err := discDeps.Matches.RunOnce(ctx); err != nil {
			log.Error("one-shot failed", "err", err)
			os.Exit(1)
		}
		log.Info("one-shot done")
		return
	}

	scheduler := discovery.NewScheduler(discDeps.Cycles, log)
	scheduler.Run(ctx)
}

func must(log *slog.Logger, what string, err error) {
	if err != nil {
		log.Error(what, "err", err)
		os.Exit(1)
	}
}