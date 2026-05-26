package features

import (
	"context"

	"github.com/user-for-download/dota2-analysis/go-analysis/internal/domain"
)

// ──────────────────────────────────────────────
// DraftSlotNormSource — slot / 30.0 (same for all candidates in a decision)
// ──────────────────────────────────────────────

type DraftSlotNormSource struct{}

func NewDraftSlotNormSource() *DraftSlotNormSource {
	return &DraftSlotNormSource{}
}

func (s *DraftSlotNormSource) Def() domain.FeatureDef {
	return domain.FeatureDef{
		Name:       "draft_slot_norm",
		Dtype:      "f32",
		SourceHash: hashOf("draft_slot_norm: slot/max_slot normalized to [0,1]"),
	}
}

func (s *DraftSlotNormSource) Compute(_ context.Context, st *domain.DraftState, candidates []domain.HeroID) (map[domain.HeroID]float64, error) {
	slotNorm := float64(st.Slot()) / 30.0
	result := make(map[domain.HeroID]float64, len(candidates))
	for _, h := range candidates {
		result[h] = slotNorm
	}
	return result, nil
}

// ──────────────────────────────────────────────
// IsPickPhaseSource — 1.0 for picks, 0.0 for bans (same for all candidates)
// ──────────────────────────────────────────────

type IsPickPhaseSource struct{}

func NewIsPickPhaseSource() *IsPickPhaseSource {
	return &IsPickPhaseSource{}
}

func (s *IsPickPhaseSource) Def() domain.FeatureDef {
	return domain.FeatureDef{
		Name:       "is_pick_phase",
		Dtype:      "f32",
		SourceHash: hashOf("is_pick_phase: 1.0 for picks, 0.0 for bans"),
	}
}

func (s *IsPickPhaseSource) Compute(_ context.Context, st *domain.DraftState, candidates []domain.HeroID) (map[domain.HeroID]float64, error) {
	var isPick float64
	if !st.IsBanPhase() {
		isPick = 1.0
	}
	result := make(map[domain.HeroID]float64, len(candidates))
	for _, h := range candidates {
		result[h] = isPick
	}
	return result, nil
}
