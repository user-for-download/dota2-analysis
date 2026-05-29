package linear

import (
	"context"

	"github.com/user-for-download/dota2-analysis/go-analysis/internal/domain"
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
//
// All 24 features from the spec MUST have non-zero weights.  Go map lookups
// silently return 0.0 for missing keys, which would cause hero priors and
// semantic draft features to be completely ignored (flatlining cold-start
// rankings when MV-dependent features 0-7 are constant across candidates).
func defaultWeights() map[string]float64 {
	return map[string]float64{
		// ── MV-dependent (0-7) — primary signals when MVs populated ────
		"team_picks":              0.002, // ⚠️ 0.05 was 25× too high for scale 0-50+ (dominates all other features).
		"team_wr_shrunk":          0.30,  // team comfort is important
		"mean_syn_with_allies":    0.25,  // synergy with allies matters
		"mean_counter_vs_enemies": 0.20,  // countering enemies matters
		"hero_meta_primary_attr":  0.02,  // slight variance for cold starts
		"hero_meta_role_count":    0.05,  // slight variance for cold starts
		"player_comfort":          0.15,  // player skill with hero
		"star_threat":             -0.10, // negative: avoid opponent's signature heroes

		// ── Hero priors (8-10) — VARY per hero, primary ranking signal ──
		"hero_pick_rate":    0.08, // pick frequency signals viability
		"hero_wr":           0.12, // global win rate matters
		"hero_popularity":   0.02, // log-scaled pick count, long-tail signal

		// ── Attribute / composition (11-14) — VARY per hero ────────────
		"attr_is_str":       0.005, // very weak — composition nuance only
		"attr_is_agi":       0.005, // very weak
		"attr_is_int":       0.005, // very weak
		"attr_fit_score":    0.04,  // composition fit, amplifies with draft depth (0-4)

		// ── Draft position (15-16) — same across all candidates in group ─
		"draft_slot_norm":   0.005, // very weak group-level signal
		"is_pick_phase":     0.01,  // picks vs bans matters a bit

		// ── Semantic draft context (17-23) — same across group ─────────
		"team_picks_before":    0.005,
		"enemy_picks_before":  -0.005, // more enemy picks → harder to counter
		"is_first_pick":        0.02,  // first pick has higher value
		"is_last_pick":         0.005,
		"is_counter_phase":     0.005,
		"remaining_team_picks": 0.005,
		"draft_progress":      -0.005, // later in draft, picks matter slightly less
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
