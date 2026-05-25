package lgbm

import (
	"context"
	"fmt"

	"github.com/user-for-download/go-dota2-analysis/internal/domain"
)

// Scorer implements scoring.Scorer using a LightGBM model.
type Scorer struct {
	ens  Model
	spec *domain.FeatureSpec
	meta ModelMeta
	dir  string
}

// Dir returns the model directory.
func (s *Scorer) Dir() string { return s.dir }

func (s *Scorer) Spec() *domain.FeatureSpec { return s.spec }
func (s *Scorer) Version() string            { return s.meta.Version }

// Score computes LightGBM predictions for a batch of feature vectors.
func (s *Scorer) Score(_ context.Context, vectors []*domain.FeatureVector) ([]domain.Score, error) {
	if len(vectors) == 0 {
		return nil, nil
	}

	// Validate spec match
	if !vectors[0].Spec().Equal(s.spec) {
		expected := "<nil>"
		if s.spec != nil {
			expected = s.spec.Version
		}
		return nil, fmt.Errorf("vector spec mismatch: expected %s, got %s", expected, vectors[0].Spec().Version)
	}

	n := len(vectors)
	nFeat := len(s.spec.Features)

	// Flatten vectors into a 2D array (row-major)
	flat := make([]float64, n*nFeat)
	for i, v := range vectors {
		vals := v.Values()
		copy(flat[i*nFeat:(i+1)*nFeat], vals)
	}

	// Batch predict using PredictDense for efficient multi-row inference
	predictions := make([]float64, n)
	if err := s.ens.PredictDense(flat, n, nFeat, predictions, 0, 1); err != nil {
		return nil, fmt.Errorf("lightgbm predict: %w", err)
	}

	scores := make([]domain.Score, n)
	for i := range vectors {
		scores[i] = domain.Score{Hero: vectors[i].Hero(), Value: predictions[i]}
	}
	return scores, nil
}

// Reload loads a fresh model from the same directory.
// Returns a new Scorer instance (caller should swap via atomic.Pointer).
func (s *Scorer) Reload() (*Scorer, error) {
	return LoadModel(s.dir)
}
