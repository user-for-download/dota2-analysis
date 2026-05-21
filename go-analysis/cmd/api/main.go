package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/user-for-download/go-dota2-analysis/internal/api"
	"github.com/user-for-download/go-dota2-analysis/internal/bootstrap"
	"github.com/user-for-download/go-dota2-analysis/internal/config"
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

	// Wire up recommendation service
	catalog, err := features.NewPGCatalog(ctx, deps.DB)
	must(log, "catalog", err)

	repo := postgres.NewPGRepository(deps.DB)

	// Wire up feature sources (one per feature dimension).
	sources := []features.FeatureSource{
		features.NewTeamPicksSource(repo),
		features.NewTeamWRShrunkSource(repo),
		features.NewSynergySource(repo),
		features.NewCounterSource(repo),
		features.NewHeroMetaPrimaryAttrSource(catalog),
		features.NewHeroMetaRoleCountSource(catalog),
		features.NewPlayerComfortSource(repo),
		features.NewStarThreatSource(repo),
	}
	builder := features.NewBuilder(sources)
	scorer := linear.NewScorer(builder.Spec())
	var explainer scoring.Explainer = linear.NewExplainer(scorer)

	// Conditionally load LGBM model for hot-reload support
	var lgbmScorer *lgbm.ReloadableScorer
	if cfg.Analytics.ScorerKind == "lgbm" {
		model, err := lgbm.LoadModel(cfg.Analytics.ModelDir)
		if err != nil {
			log.Error("failed to load LGBM model, falling back to linear", "err", err)
		} else {
			lgbmScorer = lgbm.NewReloadableScorer(model)
			// Use honest LGBM explainer (generic explanation — leaf contributions not available)
			explainer = lgbm.NewExplainer()
			log.Info("LGBM model loaded", "version", model.Version())
		}
	}

	recommender := recommend.NewService(builder, scorer, explainer, catalog)

	srv := api.NewServer(cfg.API, cfg.Analytics, repo, recommender, catalog, lgbmScorer, log)
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
