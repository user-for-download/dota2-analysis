package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/user-for-download/go-dota2-analysis/internal/bootstrap"
	"github.com/user-for-download/go-dota2-analysis/internal/config"
	"github.com/user-for-download/go-dota2-analysis/internal/featurize"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildDate = ""
)

func main() {
	log := bootstrap.NewLogger(slog.NewJSONHandler(os.Stdout, nil))
	log.Info("starting featurizer", "version", version, "commit", commit)

	cfg, err := config.Load("")
	must(log, "config", err)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdownTelemetry, err := bootstrap.InitTelemetry(ctx, "go-dota2-analysis-featurizer", cfg.Telemetry.Endpoint, cfg.Telemetry.SampleRate)
	if err != nil {
		log.Error("init telemetry", "err", err)
	} else if shutdownTelemetry != nil {
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = shutdownTelemetry(shutdownCtx)
		}()
	}

	deps, err := bootstrap.Core(ctx, cfg, log)
	must(log, "bootstrap", err)
	defer deps.Close()

	interval := cfg.Analytics.FeaturizerInterval
	if interval <= 0 {
		interval = 24 * time.Hour
	}

	runner := featurize.NewRunner(deps.DB, interval, log)

	log.Info("featurizer running", "interval", interval.String())
	if err := runner.Run(ctx); err != nil && err != context.Canceled {
		log.Error("featurizer exited with error", "err", err)
		os.Exit(1)
	}

	log.Info("featurizer shut down gracefully")
}

func must(log *slog.Logger, what string, err error) {
	if err != nil {
		log.Error(what, "err", err)
		os.Exit(1)
	}
}
