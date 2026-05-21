package scoring

import (
	"context"

	"github.com/user-for-download/go-dota2-analysis/internal/domain"
)

// Explainer decomposes a score into human-readable contributing factors.
type Explainer interface {
	// Explain returns reasons supporting the score and risks detracting from it.
	Explain(ctx context.Context, hero domain.HeroID, score float64) ([]domain.Reason, []domain.Reason, error)
}
