package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/user-for-download/dota2-analysis/go-analysis/internal/config"
	"github.com/user-for-download/dota2-analysis/go-analysis/internal/domain"
	"github.com/user-for-download/dota2-analysis/go-analysis/internal/profiles"
	"github.com/user-for-download/dota2-analysis/go-analysis/internal/recommend"
	"github.com/user-for-download/dota2-analysis/go-analysis/internal/scoring/lgbm"
)

// Handler holds dependencies for HTTP handlers.
type Handler struct {
	repo        profiles.Repository
	analytics   config.AnalyticsConfig
	log         *slog.Logger
	recommender recommend.Recommender
	catalog     domain.HeroCatalog
	lgbmScorer  *lgbm.ReloadableScorer // nil if using linear scorer
}

// NewHandler creates a new Handler.
func NewHandler(repo profiles.Repository, analytics config.AnalyticsConfig, recommender recommend.Recommender, catalog domain.HeroCatalog, lgbmScorer *lgbm.ReloadableScorer, log *slog.Logger) *Handler {
	return &Handler{repo: repo, analytics: analytics, recommender: recommender, catalog: catalog, lgbmScorer: lgbmScorer, log: log}
}

// HealthResponse is the JSON structure for /v1/health.
type HealthResponse struct {
	Status                   string     `json:"status"`
	PatchID                  int32      `json:"patch_id"`
	Scorer                   string     `json:"scorer"`
	ModelVersion             string     `json:"model_version"`
	LastFeaturizerSuccess    time.Time  `json:"last_featurizer_success"`
	FeaturizerStalenessHours *float64   `json:"featurizer_staleness_hours,omitempty"`
}

// Health returns the health status including featurizer staleness.
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	statusRec, err := h.repo.FeaturizerStatus(r.Context())
	lastSuccess := statusRec.LastSuccessful

	var staleness *float64
	status := "ok"
	if err == nil && !lastSuccess.IsZero() {
		v := time.Since(lastSuccess).Hours()
		staleness = &v
		if v > 36 {
			status = "stale"
		}
	} else {
		status = "stale"
		// staleness stays nil when no data is available
	}

	scorerKind := "linear"
	modelVersion := ""
	if h.lgbmScorer != nil {
		scorerKind = "lgbm"
		modelVersion = h.lgbmScorer.Version()
	}

	h.writeJSON(w, HealthResponse{
		Status:                   status,
		PatchID:                  h.analytics.CurrentPatchID,
		Scorer:                   scorerKind,
		ModelVersion:             modelVersion,
		LastFeaturizerSuccess:    lastSuccess,
		FeaturizerStalenessHours: staleness,
	})
}

// writeJSON encodes v as JSON and logs any encode error.
func (h *Handler) writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		h.log.Error("json encode failed", "err", err)
	}
}
