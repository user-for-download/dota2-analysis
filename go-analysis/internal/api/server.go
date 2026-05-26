package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/user-for-download/dota2-analysis/go-analysis/internal/config"
	"github.com/user-for-download/dota2-analysis/go-analysis/internal/domain"
	"github.com/user-for-download/dota2-analysis/go-analysis/internal/profiles"
	"github.com/user-for-download/dota2-analysis/go-analysis/internal/recommend"
	"github.com/user-for-download/dota2-analysis/go-analysis/internal/scoring"
)

// Server is the HTTP API server.
type Server struct {
	apiCfg    config.APIConfig
	analytics config.AnalyticsConfig
	handler   *Handler
	log       *slog.Logger
	reloader  scoring.ModelReloader // nil if using linear scorer
}

// NewServer creates a new API server.
func NewServer(apiCfg config.APIConfig, analytics config.AnalyticsConfig, repo profiles.Repository, recommender recommend.Recommender, catalog domain.HeroCatalog, reloader scoring.ModelReloader, log *slog.Logger) *Server {
	return &Server{
		apiCfg:    apiCfg,
		analytics: analytics,
		handler:   NewHandler(repo, analytics, recommender, catalog, reloader, log),
		log:       log,
		reloader:  reloader,
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

	// API routes with auth middleware
	mux.Handle("/v1/", AuthMiddleware(s.apiCfg.Token)(apiMux))

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
