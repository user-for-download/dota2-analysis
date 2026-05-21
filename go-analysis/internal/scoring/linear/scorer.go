package linear

import (
	"context"

	"github.com/user-for-download/go-dota2-analysis/internal/domain"
)

// Scorer computes a weighted sum of feature values.
type Scorer struct {
	spec    *domain.FeatureSpec
	weights map[string]float64
}

// NewScorer creates a linear scorer with the given feature spec and default
// hand-tuned weights. The spec must match the builder that produced the vectors.
func NewScorer(spec *domain.FeatureSpec) *Scorer {
	return &Scorer{
		spec:    spec,
		weights: defaultWeights(),
	}
}

// defaultWeights returns the hand-tuned weight map for each feature.
// Positive weights indicate desirable traits; negative weights penalize.
// Calibrated against backtest 2025-10-15 patch 71: R@5=0.42, NDCG@10=0.51.
// Re-calibrate whenever feature definitions change (bump FeatureSpecVersion).
func defaultWeights() map[string]float64 {
	return map[string]float64{
		"team_picks":              0.05,  // slight preference for familiar picks
		"team_wr_shrunk":          0.30,  // team comfort is important
		"mean_syn_with_allies":    0.25,  // synergy with allies matters
		"mean_counter_vs_enemies": 0.20,  // countering enemies matters
		"hero_meta_primary_attr":  0.02,  // slight variance for cold starts
		"hero_meta_role_count":    0.05,  // slight variance for cold starts
		"player_comfort":          0.15,  // player skill with hero
		"star_threat":             -0.10, // negative: avoid opponent's signature heroes
	}
}

func (s *Scorer) Spec() *domain.FeatureSpec { return s.spec }
func (s *Scorer) Version() string            { return "linear-v1" }

// Score computes a weighted sum for each feature vector.
func (s *Scorer) Score(_ context.Context, vectors []*domain.FeatureVector) ([]domain.Score, error) {
	out := make([]domain.Score, len(vectors))
	for i, v := range vectors {
		val := 0.5 // baseline prevents 0.0 flatline when DB features are sparse
		vals := v.Values()
		for j, def := range s.spec.Features {
			w := s.weights[def.Name]
			val += w * vals[j]
		}
		out[i] = domain.Score{Hero: v.Hero(), Value: val}
	}
	return out, nil
}

// Weights returns a copy of the current weights map.
func (s *Scorer) Weights() map[string]float64 {
	out := make(map[string]float64, len(s.weights))
	for k, v := range s.weights {
		out[k] = v
	}
	return out
}

// SetWeight updates a single feature weight.
func (s *Scorer) SetWeight(name string, w float64) {
	s.weights[name] = w
}
