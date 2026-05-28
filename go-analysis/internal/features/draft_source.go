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

// ────────────────────────────────────────────────────────────
// Semantic draft context features (patch-invariant relative state)
// ────────────────────────────────────────────────────────────

// teamPicksBefore returns the number of picks by the acting team before this slot.
// DraftState is always constructed with userTeam = next actor, so AllyPicks()
// is always the acting team's picks. See handlers_draft.go line 60-63.
func teamPicksBefore(st *domain.DraftState) int {
	return len(st.AllyPicks())
}

// enemyPicksBefore returns the number of picks by the non-acting team.
// See teamPicksBefore — EnemyPicks() is always the other team.
func enemyPicksBefore(st *domain.DraftState) int {
	return len(st.EnemyPicks())
}

// ── TeamPicksBeforeSource ──────────────────────────────────

type TeamPicksBeforeSource struct{}

func NewTeamPicksBeforeSource() *TeamPicksBeforeSource {
	return &TeamPicksBeforeSource{}
}

func (s *TeamPicksBeforeSource) Def() domain.FeatureDef {
	return domain.FeatureDef{
		Name:       "team_picks_before",
		Dtype:      "f32",
		SourceHash: hashOf("team_picks_before: picks by acting_team before this slot"),
	}
}

func (s *TeamPicksBeforeSource) Compute(_ context.Context, st *domain.DraftState, candidates []domain.HeroID) (map[domain.HeroID]float64, error) {
	v := float64(teamPicksBefore(st))
	result := make(map[domain.HeroID]float64, len(candidates))
	for _, h := range candidates {
		result[h] = v
	}
	return result, nil
}

// ── EnemyPicksBeforeSource ─────────────────────────────────

type EnemyPicksBeforeSource struct{}

func NewEnemyPicksBeforeSource() *EnemyPicksBeforeSource {
	return &EnemyPicksBeforeSource{}
}

func (s *EnemyPicksBeforeSource) Def() domain.FeatureDef {
	return domain.FeatureDef{
		Name:       "enemy_picks_before",
		Dtype:      "f32",
		SourceHash: hashOf("enemy_picks_before: picks by enemy team before this slot"),
	}
}

func (s *EnemyPicksBeforeSource) Compute(_ context.Context, st *domain.DraftState, candidates []domain.HeroID) (map[domain.HeroID]float64, error) {
	v := float64(enemyPicksBefore(st))
	result := make(map[domain.HeroID]float64, len(candidates))
	for _, h := range candidates {
		result[h] = v
	}
	return result, nil
}

// ── IsFirstPickSource ─────────────────────────────────────

type IsFirstPickSource struct{}

func NewIsFirstPickSource() *IsFirstPickSource {
	return &IsFirstPickSource{}
}

func (s *IsFirstPickSource) Def() domain.FeatureDef {
	return domain.FeatureDef{
		Name:       "is_first_pick",
		Dtype:      "f32",
		SourceHash: hashOf("is_first_pick: team_picks_before == 0"),
	}
}

func (s *IsFirstPickSource) Compute(_ context.Context, st *domain.DraftState, candidates []domain.HeroID) (map[domain.HeroID]float64, error) {
	v := 0.0
	if teamPicksBefore(st) == 0 {
		v = 1.0
	}
	result := make(map[domain.HeroID]float64, len(candidates))
	for _, h := range candidates {
		result[h] = v
	}
	return result, nil
}

// ── IsLastPickSource ──────────────────────────────────────

type IsLastPickSource struct{}

func NewIsLastPickSource() *IsLastPickSource {
	return &IsLastPickSource{}
}

func (s *IsLastPickSource) Def() domain.FeatureDef {
	return domain.FeatureDef{
		Name:       "is_last_pick",
		Dtype:      "f32",
		SourceHash: hashOf("is_last_pick: team_picks_before == 4 (5th pick)"),
	}
}

func (s *IsLastPickSource) Compute(_ context.Context, st *domain.DraftState, candidates []domain.HeroID) (map[domain.HeroID]float64, error) {
	v := 0.0
	if teamPicksBefore(st) == 4 {
		v = 1.0
	}
	result := make(map[domain.HeroID]float64, len(candidates))
	for _, h := range candidates {
		result[h] = v
	}
	return result, nil
}

// ── IsCounterPhaseSource ──────────────────────────────────

type IsCounterPhaseSource struct{}

func NewIsCounterPhaseSource() *IsCounterPhaseSource {
	return &IsCounterPhaseSource{}
}

func (s *IsCounterPhaseSource) Def() domain.FeatureDef {
	return domain.FeatureDef{
		Name:       "is_counter_phase",
		Dtype:      "f32",
		SourceHash: hashOf("is_counter_phase: enemy_picks_before > team_picks_before"),
	}
}

func (s *IsCounterPhaseSource) Compute(_ context.Context, st *domain.DraftState, candidates []domain.HeroID) (map[domain.HeroID]float64, error) {
	v := 0.0
	if enemyPicksBefore(st) > teamPicksBefore(st) {
		v = 1.0
	}
	result := make(map[domain.HeroID]float64, len(candidates))
	for _, h := range candidates {
		result[h] = v
	}
	return result, nil
}

// ── RemainingTeamPicksSource ──────────────────────────────

type RemainingTeamPicksSource struct{}

func NewRemainingTeamPicksSource() *RemainingTeamPicksSource {
	return &RemainingTeamPicksSource{}
}

func (s *RemainingTeamPicksSource) Def() domain.FeatureDef {
	return domain.FeatureDef{
		Name:       "remaining_team_picks",
		Dtype:      "f32",
		SourceHash: hashOf("remaining_team_picks: 5 - team_picks_before"),
	}
}

func (s *RemainingTeamPicksSource) Compute(_ context.Context, st *domain.DraftState, candidates []domain.HeroID) (map[domain.HeroID]float64, error) {
	v := float64(max(0, 5-teamPicksBefore(st)))
	result := make(map[domain.HeroID]float64, len(candidates))
	for _, h := range candidates {
		result[h] = v
	}
	return result, nil
}

// ── DraftProgressSource ──────────────────────────────────

type DraftProgressSource struct{}

func NewDraftProgressSource() *DraftProgressSource {
	return &DraftProgressSource{}
}

func (s *DraftProgressSource) Def() domain.FeatureDef {
	return domain.FeatureDef{
		Name:       "draft_progress",
		Dtype:      "f32",
		SourceHash: hashOf("draft_progress: (team + enemy picks) / 10"),
	}
}

func (s *DraftProgressSource) Compute(_ context.Context, st *domain.DraftState, candidates []domain.HeroID) (map[domain.HeroID]float64, error) {
	totalPicks := teamPicksBefore(st) + enemyPicksBefore(st)
	v := float64(totalPicks) / 10.0
	if v > 1.0 {
		v = 1.0
	}
	result := make(map[domain.HeroID]float64, len(candidates))
	for _, h := range candidates {
		result[h] = v
	}
	return result, nil
}
