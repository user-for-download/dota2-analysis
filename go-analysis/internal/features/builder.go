package features

import (
	"context"
	"fmt"

	"github.com/user-for-download/dota2-analysis/go-analysis/internal/domain"
)

// Builder assembles FeatureVectors from a DraftState and a set of candidate heroes.
type Builder interface {
	Spec() *domain.FeatureSpec
	Build(ctx context.Context, st *domain.DraftState, candidates []domain.HeroID) ([]*domain.FeatureVector, error)
}

// sourceBuilder implements Builder by composing FeatureSource values.
type sourceBuilder struct {
	sources []FeatureSource
	spec    *domain.FeatureSpec
}

// NewBuilder creates a Builder from the given feature sources.
// The FeatureSpec is derived from the sources — they can never disagree.
func NewBuilder(sources []FeatureSource) Builder {
	defs := make([]domain.FeatureDef, len(sources))
	for i, s := range sources {
		defs[i] = s.Def()
	}
	return &sourceBuilder{
		sources: sources,
		spec: &domain.FeatureSpec{
			Version:  FeatureSpecVersion,
			Features: defs,
		},
	}
}

// Spec returns the feature spec this builder produces.
func (b *sourceBuilder) Spec() *domain.FeatureSpec { return b.spec }

// Build computes feature vectors for each candidate hero given the current draft state.
func (b *sourceBuilder) Build(ctx context.Context, st *domain.DraftState, candidates []domain.HeroID) ([]*domain.FeatureVector, error) {
	if len(candidates) == 0 {
		return nil, nil
	}

	nFeat := len(b.sources)
	values := make(map[domain.HeroID][]float64, len(candidates))

	// Compute all source values, one dimension per source.
	for i, src := range b.sources {
		result, err := src.Compute(ctx, st, candidates)
		if err != nil {
			return nil, fmt.Errorf("compute %s: %w", src.Def().Name, err)
		}

		// Assign this source's values to the i-th slot for each candidate.
		if i == 0 {
			// First pass: allocate vectors.
			for _, h := range candidates {
				values[h] = make([]float64, nFeat)
			}
		}
		for _, h := range candidates {
			values[h][i] = result[h]
		}
	}

	out := make([]*domain.FeatureVector, len(candidates))
	for i, h := range candidates {
		out[i] = domain.NewFeatureVector(b.spec, h, values[h])
	}
	return out, nil
}
