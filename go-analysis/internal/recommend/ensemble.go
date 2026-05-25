package recommend

import (
	"github.com/user-for-download/dota2-analysis/go-analysis/internal/domain"
)

// EnsembleScore combines imitation and value model scores.
// In Week 1-4, this is a passthrough. Week 5 adds value model weighting.
type EnsembleScore struct {
	ImitationWeight float64
	ValueWeight     float64
}

// DefaultEnsemble returns a 100% imitation ensemble (value model not yet trained).
func DefaultEnsemble() *EnsembleScore {
	return &EnsembleScore{
		ImitationWeight: 1.0,
		ValueWeight:     0.0,
	}
}

// BalancedEnsemble returns a 70/30 imitation/value split.
func BalancedEnsemble() *EnsembleScore {
	return &EnsembleScore{
		ImitationWeight: 0.7,
		ValueWeight:     0.3,
	}
}

// Combine merges imitation and value scores into a final score.
func (e *EnsembleScore) Combine(imitation, value []domain.Score) []domain.Score {
	if e.ValueWeight == 0 || len(value) == 0 {
		return imitation
	}

	valueMap := make(map[domain.HeroID]float64, len(value))
	for _, s := range value {
		valueMap[s.Hero] = s.Value
	}

	out := make([]domain.Score, len(imitation))
	for i, s := range imitation {
		v, ok := valueMap[s.Hero]
		if !ok {
			// Hero not scored by value model — use imitation score only.
			v = s.Value
		}
		out[i] = domain.Score{
			Hero:  s.Hero,
			Value: e.ImitationWeight*s.Value + e.ValueWeight*v,
		}
	}
	return out
}
