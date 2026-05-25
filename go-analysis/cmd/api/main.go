package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/user-for-download/go-dota2-analysis/internal/api"
	"github.com/user-for-download/go-dota2-analysis/internal/bootstrap"
	"github.com/user-for-download/go-dota2-analysis/internal/config"
	"github.com/user-for-download/go-dota2-analysis/internal/domain"
	"github.com/user-for-download/go-dota2-analysis/internal/features"
	"github.com/user-for-download/go-dota2-analysis/internal/recommend"
	"github.com/user-for-download/go-dota2-analysis/internal/scoring"
	"github.com/user-for-download/go-dota2-analysis/internal/scoring/lgbm"
	"github.com/user-for-download/go-dota2-analysis/internal/scoring/linear"
	"github.com/user-for-download/go-dota2-analysis/internal/storage/postgres"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildDate = ""
)

func main() {
	log := bootstrap.NewLogger(slog.NewJSONHandler(os.Stdout, nil))
	log.Info("starting api", "version", version, "commit", commit)

	cfg, err := config.Load("")
	must(log, "config", err)
	if cfg.Analytics.CurrentPatchID == 0 {
		log.Error("ANALYTICS_PATCH_ID must be set to a non-zero value")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdownTelemetry, err := bootstrap.InitTelemetry(ctx, "go-dota2-analysis-api", cfg.Telemetry.Endpoint, cfg.Telemetry.SampleRate)
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

	// Wire up recommendation service
	catalog, err := features.NewPGCatalog(ctx, deps.DB)
	must(log, "catalog", err)

	repo := postgres.NewPGRepository(deps.DB)

	// Feature registry: decouples Go source code from ML feature ordering.
	// The ML model's spec.json drives which features are computed and in what
	// order — no hardcoded slice, no manual reordering when the model changes.
	featReg := features.DefaultRegistry()

	var (
		builder    features.Builder
		scorer     scoring.Scorer
		explainer  scoring.Explainer
		lgbmScvr   *lgbm.ReloadableScorer
		linearScor *linear.Scorer // concrete type needed for linear.NewExplainer
	)

	if cfg.Analytics.ScorerKind == "lgbm" {
		// Dynamic path: read spec.json from the model directory and wire
		// features by name from the registry.  This lets the ML team add,
		// remove, or reorder features without a Go recompile.
		specData, err := os.ReadFile(filepath.Join(cfg.Analytics.ModelDir, "spec.json"))
		if err != nil {
			log.Warn("could not read spec.json, falling back to default order", "err", err)
			builder = features.NewBuilder(features.DefaultSources(repo, catalog))
		} else {
			var spec domain.FeatureSpec
			if uErr := json.Unmarshal(specData, &spec); uErr != nil {
				log.Warn("could not parse spec.json, falling back to default order", "err", uErr)
				builder = features.NewBuilder(features.DefaultSources(repo, catalog))
			} else {
				b, bErr := features.NewBuilderFromSpec(&spec, repo, catalog, featReg)
				if bErr != nil {
					log.Warn("could not resolve spec.json features, falling back to default order", "err", bErr)
					builder = features.NewBuilder(features.DefaultSources(repo, catalog))
				} else {
					builder = b
					log.Info("feature builder configured from spec.json", "features", len(spec.Features))
				}
			}
		}

		model, err := lgbm.LoadModel(cfg.Analytics.ModelDir)
		if err != nil {
			log.Error("failed to load LGBM model, falling back to linear", "err", err)
			linearScor = linear.NewScorer(builder.Spec())
			scorer = linearScor
			explainer = linear.NewExplainer(linearScor)
		} else {
			lgbmScvr = lgbm.NewReloadableScorer(model)
			scorer = lgbmScvr
			explainer = lgbm.NewExplainer()
			log.Info("LGBM model loaded", "version", model.Version())
		}
	} else {
		// Linear scorer path: use default feature ordering.
		builder = features.NewBuilder(features.DefaultSources(repo, catalog))
		linearScor = linear.NewScorer(builder.Spec())
		scorer = linearScor
		explainer = linear.NewExplainer(linearScor)
	}

	if scorer == nil {
		// Guard: nil scorer if LGBM load failed without falling through to
		// the linear path above (e.g. ScorerKind == "lgbm" but builder
		// succeeded and model load returned nil without error, or similar
		// edge cases).  Should not happen in practice, but be safe.
		linearScor = linear.NewScorer(builder.Spec())
		scorer = linearScor
		explainer = linear.NewExplainer(linearScor)
	}

	recommender := recommend.NewService(builder, scorer, explainer, catalog)

	srv := api.NewServer(cfg.API, cfg.Analytics, repo, recommender, catalog, lgbmScvr, log)
	if err := srv.Run(ctx); err != nil {
		log.Error("server", "err", err)
		os.Exit(1)
	}
}

func must(log *slog.Logger, what string, err error) {
	if err != nil {
		log.Error(what, "err", err)
		os.Exit(1)
	}
}
