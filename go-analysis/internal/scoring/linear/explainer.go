package linear

import (
	"context"
	"fmt"

	"github.com/user-for-download/dota2-analysis/go-analysis/internal/domain"
)

// Explainer decomposes a linear score into per-hero feature contributions.
type Explainer struct {
	scorer *Scorer
}

// NewExplainer creates an explainer bound to the given scorer.
func NewExplainer(scorer *Scorer) *Explainer {
	return &Explainer{scorer: scorer}
}

// Explain returns reasons and risks based on the hero's actual feature values.
// Each reason/risk contribution is computed as weight × feature_value, so
// the output is specific to the hero instead of regurgitating global weights.
func (e *Explainer) Explain(_ context.Context, vector *domain.FeatureVector, score float64) ([]domain.Reason, []domain.Reason, error) {
	weights := e.scorer.Weights()
	spec := vector.Spec()
	vals := vector.Values()

	var reasons []domain.Reason
	var risks []domain.Reason

	for j, def := range spec.Features {
		w := weights[def.Name]
		contribution := w * vals[j]
		switch {
		case contribution > 0.1:
			reasons = append(reasons, domain.Reason{
				Factor: def.Name,
				Note:   fmt.Sprintf("contribution: %.4f", contribution),
				Delta:  contribution,
			})
		case contribution < -0.05:
			risks = append(risks, domain.Reason{
				Factor: def.Name,
				Note:   fmt.Sprintf("contribution: %.4f", contribution),
				Delta:  contribution,
			})
		}
	}

	return reasons, risks, nil
}
