package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/user-for-download/dota2-analysis/go-analysis/internal/bootstrap"
	"github.com/user-for-download/dota2-analysis/go-analysis/internal/config"
	"github.com/user-for-download/dota2-analysis/go-analysis/internal/eval"
	"github.com/user-for-download/dota2-analysis/go-analysis/internal/features"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildDate = ""
)

func main() {
	log := bootstrap.NewLogger(slog.NewJSONHandler(os.Stdout, nil))
	log.Info("starting backtester", "version", version, "commit", commit)

	cfg, err := config.Load("")
	must(log, "config", err)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdownTelemetry, err := bootstrap.InitTelemetry(ctx, "go-dota2-analysis-backtester", cfg.Telemetry.Endpoint, cfg.Telemetry.SampleRate)
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

	// Block until featurizer has populated the materialized views
	if err := bootstrap.WaitForLaunchKey(ctx, deps.DB, "featurizer_ready", log); err != nil {
		log.Error("wait for featurizer", "err", err)
		os.Exit(1)
	}

	// Determine patch ID: env var or config.
	patchID := getPatchID(cfg, log)
	if patchID == 0 {
		log.Error("backtester: ANALYTICS_PATCH_ID or BACKTEST_PATCH_ID is required")
		os.Exit(1)
	}

	limit := getLimit()

	catalog, err := features.NewPGCatalog(ctx, deps.DB)
	must(log, "load hero catalog", err)
	log.Info("hero catalog loaded", "heroes", len(catalog.All()))

	// Build baselines map after loading.
	baselines := map[string]eval.Baseline{
		"uniform_random": eval.UniformRandomBaseline{},
	}

	// Load pick frequency baseline.
	pfBaseline, err := eval.NewPickFrequencyBaseline(ctx, deps.DB, patchID)
	if err != nil {
		log.Error("load pick frequency baseline", "err", err)
	} else {
		baselines["pick_frequency"] = pfBaseline
		log.Info("pick frequency baseline loaded", "heroes_with_picks", len(pfBaseline.Freqs))
	}

	cfgReplay := eval.ReplayConfig{
		PatchID: patchID,
		Limit:   limit,
	}

	for name, baseline := range baselines {
		log.Info("running backtest", "baseline", name, "patch", patchID, "limit", limit)
		result, err := eval.Replay(ctx, deps.DB, catalog, baseline, cfgReplay)
		if err != nil {
			log.Error("replay failed", "baseline", name, "err", err)
			continue
		}

		printResult(log, name, result)
	}

	// Player comfort baseline — uses player-level win rate data.
	playerBaseline := eval.NewPlayerComfortBaseline(deps.DB)
	log.Info("running backtest", "baseline", "player_comfort", "patch", patchID, "limit", limit)
	playerResult, err := eval.Replay(ctx, deps.DB, catalog, playerBaseline, cfgReplay)
	if err != nil {
		log.Error("player comfort baseline", "err", err)
	} else {
		printResult(log, "player_comfort", playerResult)
	}
}

// getPatchID reads the patch ID from BACKTEST_PATCH_ID or falls back to config.
func getPatchID(cfg *config.Config, log *slog.Logger) int32 {
	if v := os.Getenv("BACKTEST_PATCH_ID"); v != "" {
		n, err := strconv.ParseInt(v, 10, 32)
		if err != nil {
			log.Error("invalid BACKTEST_PATCH_ID", "value", v, "err", err)
			return 0
		}
		return int32(n)
	}
	return cfg.Analytics.CurrentPatchID
}

// getLimit reads the max match limit from BACKTEST_LIMIT (0 = all).
func getLimit() int {
	if v := os.Getenv("BACKTEST_LIMIT"); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil && n > 0 {
			return n
		}
	}
	return 0
}

// printResult outputs the backtest result as structured JSON to stdout.
func printResult(log *slog.Logger, name string, result *eval.BacktestResult) {
	out := struct {
		Baseline string              `json:"baseline"`
		Overall  eval.PhaseMetrics   `json:"overall"`
		PerPhase []eval.PhaseMetrics `json:"per_phase"`
	}{
		Baseline: name,
		Overall:  result.Overall,
		PerPhase: result.PerPhase,
	}

	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		log.Error("marshal result", "err", err)
		return
	}
	fmt.Println(string(b))
}

func must(log *slog.Logger, what string, err error) {
	if err != nil {
		log.Error(what, "err", err)
		os.Exit(1)
	}
}
