package api

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/user-for-download/dota2-analysis/go-analysis/internal/api/static"
	"github.com/user-for-download/dota2-analysis/go-analysis/internal/config"
	"github.com/user-for-download/dota2-analysis/go-analysis/internal/domain"
	"github.com/user-for-download/dota2-analysis/go-analysis/internal/profiles"
	"github.com/user-for-download/dota2-analysis/go-analysis/internal/recommend"
	"github.com/user-for-download/dota2-analysis/go-analysis/internal/scoring/lgbm"
)

// Server is the HTTP API server.
type Server struct {
	apiCfg     config.APIConfig
	analytics  config.AnalyticsConfig
	handler    *Handler
	log        *slog.Logger
	lgbmScorer *lgbm.ReloadableScorer // nil if using linear scorer
}

// NewServer creates a new API server.
func NewServer(apiCfg config.APIConfig, analytics config.AnalyticsConfig, repo profiles.Repository, recommender recommend.Recommender, catalog domain.HeroCatalog, lgbmScorer *lgbm.ReloadableScorer, log *slog.Logger) *Server {
	return &Server{
		apiCfg:     apiCfg,
		analytics:  analytics,
		handler:    NewHandler(repo, analytics, recommender, catalog, lgbmScorer, log),
		log:        log,
		lgbmScorer: lgbmScorer,
	}
}

// Run starts the HTTP server and blocks until ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	mux := http.NewServeMux()

	// API routes
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("GET /v1/health", s.handler.Health)
	apiMux.HandleFunc("POST /v1/recommend", s.handler.Recommend)
	apiMux.HandleFunc("POST /v1/draft/simulate", s.handler.SimulateDraft)
	apiMux.HandleFunc("GET /v1/teams/{id}/profile", s.handler.TeamProfile)
	apiMux.HandleFunc("GET /v1/h2h", s.handler.H2H)
	apiMux.HandleFunc("GET /v1/heroes/{id}/synergy", s.handler.HeroSynergy)
	apiMux.HandleFunc("GET /v1/heroes/{id}/counter", s.handler.HeroCounter)
	apiMux.HandleFunc("GET /v1/players/{id}/profile", s.handler.PlayerProfile)

	// Serve static UI (embedded)
	staticFS, err := fs.Sub(static.FS, ".")
	if err != nil {
		return fmt.Errorf("embed static: %w", err)
	}
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	// API routes with auth middleware
	mux.Handle("/v1/", AuthMiddleware(s.apiCfg.Token)(apiMux))

	// Route precedence: Go 1.22+ ServeMux matches most-specific path first,
	// so /v1/ takes precedence over / (static files). Static FileServer
	// returns 404 for paths that don't match embedded assets — this is
	// correct because /v1/ is handled by apiMux, not FileServer.
	var h http.Handler = mux
	h = RequestIDMiddleware(h)
	h = LoggingMiddleware(s.log)(h)

	srv := &http.Server{
		Addr:         s.apiCfg.BindAddr,
		Handler:      h,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// SIGHUP hot-reload for LGBM model (zero-downtime, atomic swap)
	if s.lgbmScorer != nil {
		sigHUP := make(chan os.Signal, 1)
		signal.Notify(sigHUP, syscall.SIGHUP)
		go func() {
			defer signal.Stop(sigHUP)
			for {
				select {
				case <-sigHUP:
					if err := s.lgbmScorer.Reload(); err != nil {
						s.log.Error("model reload failed", "err", err)
					} else {
						s.log.Info("model reloaded successfully", "version", s.lgbmScorer.Version())
					}
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	errCh := make(chan error, 1)
	go func() {
		s.log.Info("API listening", "addr", s.apiCfg.BindAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
	case err := <-errCh:
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
