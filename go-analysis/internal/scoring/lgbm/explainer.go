package lgbm

import (
	"context"
	"fmt"

	"github.com/user-for-download/dota2-analysis/go-analysis/internal/domain"
)

// Explainer provides explanations for LGBM model scores.
// LGBM leaf-contribution analysis is not available through the leaves library,
// so we return a generic explanation that honestly states the score origin.
type Explainer struct{}

// NewExplainer creates an LGBM explainer.
func NewExplainer() *Explainer {
	return &Explainer{}
}

// Explain returns reasons and risks for a given hero score.
// The explanation is generic since SHAP/leaf-contribution is not available
// through the leaves library.
func (e *Explainer) Explain(_ context.Context, vector *domain.FeatureVector, score float64) ([]domain.Reason, []domain.Reason, error) {
	return []domain.Reason{
		{Factor: "model", Note: fmt.Sprintf("score=%.4f", score)},
	}, nil, nil
}
