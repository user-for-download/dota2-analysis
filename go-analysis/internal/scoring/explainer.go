package scoring

import (
	"context"

	"github.com/user-for-download/dota2-analysis/go-analysis/internal/domain"
)

// Explainer decomposes a score into human-readable contributing factors.
type Explainer interface {
	// Explain returns reasons supporting the score and risks detracting from it.
	// The FeatureVector is needed to compute per-hero feature contributions.
	Explain(ctx context.Context, vector *domain.FeatureVector, score float64) ([]domain.Reason, []domain.Reason, error)
}
