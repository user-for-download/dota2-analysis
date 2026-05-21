package linear

import (
	"context"
	"fmt"

	"github.com/user-for-download/go-dota2-analysis/internal/domain"
)

// Explainer decomposes a linear score into per-feature contributions.
type Explainer struct {
	scorer *Scorer
}

// NewExplainer creates an explainer bound to the given scorer.
func NewExplainer(scorer *Scorer) *Explainer {
	return &Explainer{scorer: scorer}
}

// Explain returns reasons and risks based on feature weight magnitudes.
// Positive high-weight features are listed as reasons; negative-weight
// features are listed as risks.
func (e *Explainer) Explain(_ context.Context, hero domain.HeroID, score float64) ([]domain.Reason, []domain.Reason, error) {
	weights := e.scorer.Weights()

	var reasons []domain.Reason
	var risks []domain.Reason

	for name, w := range weights {
		if w > 0.1 {
			reasons = append(reasons, domain.Reason{
				Factor: name,
				Note:   fmt.Sprintf("weight: %.2f", w),
				Delta:  w,
			})
		} else if w < -0.05 {
			risks = append(risks, domain.Reason{
				Factor: name,
				Note:   fmt.Sprintf("weight: %.2f (negative)", w),
				Delta:  w,
			})
		}
	}

	return reasons, risks, nil
}
