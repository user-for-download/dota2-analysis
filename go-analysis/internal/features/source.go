package features

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/user-for-download/dota2-analysis/go-analysis/internal/domain"
	"github.com/user-for-download/dota2-analysis/go-analysis/internal/profiles"
)

// FeatureSpecVersion is the current version identifier for the feature spec.
const FeatureSpecVersion = "2026-05-26"

// FeatureSource computes one feature dimension for a set of candidate heroes.
// Each source produces exactly one float64 value per hero.
type FeatureSource interface {
	Def() domain.FeatureDef
	Compute(ctx context.Context, st *domain.DraftState, candidates []domain.HeroID) (map[domain.HeroID]float64, error)
}

// ──────────────────────────────────────────────
// TeamPicksSource — number of times the team has picked this hero
// ──────────────────────────────────────────────

type TeamPicksSource struct {
	repo profiles.Repository
}

func NewTeamPicksSource(repo profiles.Repository) *TeamPicksSource {
	return &TeamPicksSource{repo: repo}
}

func (s *TeamPicksSource) Def() domain.FeatureDef {
	return domain.FeatureDef{Name: "team_picks", Dtype: "f32", SourceHash: hashOf("team_picks: SELECT games FROM mv_team_hero_profile WHERE team_id=? AND hero_id=?")}
}

func (s *TeamPicksSource) Compute(ctx context.Context, st *domain.DraftState, candidates []domain.HeroID) (map[domain.HeroID]float64, error) {
	stats, err := s.repo.TeamHeroStatsBatch(ctx, st.TeamID(), candidates)
	if err != nil {
		return nil, err
	}
	result := make(map[domain.HeroID]float64, len(candidates))
	for _, h := range candidates {
		if stat, ok := stats[h]; ok {
			result[h] = float64(stat.Games)
		} else {
			result[h] = 0 // no data = zero games
		}
	}
	return result, nil
}

// ──────────────────────────────────────────────
// TeamWRShrunkSource — team's shrunk win rate with this hero
// ──────────────────────────────────────────────

type TeamWRShrunkSource struct {
	repo profiles.Repository
}

func NewTeamWRShrunkSource(repo profiles.Repository) *TeamWRShrunkSource {
	return &TeamWRShrunkSource{repo: repo}
}

func (s *TeamWRShrunkSource) Def() domain.FeatureDef {
	return domain.FeatureDef{Name: "team_wr_shrunk", Dtype: "f32", SourceHash: hashOf("team_wr_shrunk: SELECT wr_shrunk FROM mv_team_hero_profile WHERE team_id=? AND hero_id=?")}
}

func (s *TeamWRShrunkSource) Compute(ctx context.Context, st *domain.DraftState, candidates []domain.HeroID) (map[domain.HeroID]float64, error) {
	stats, err := s.repo.TeamHeroStatsBatch(ctx, st.TeamID(), candidates)
	if err != nil {
		return nil, err
	}
	result := make(map[domain.HeroID]float64, len(candidates))
	for _, h := range candidates {
		if stat, ok := stats[h]; ok {
			result[h] = stat.WRShrunk
		} else {
			result[h] = 0.5 // neutral prior for unseen heroes
		}
	}
	return result, nil
}

// ──────────────────────────────────────────────
// SynergySource — average synergy WR with ally picks
// ──────────────────────────────────────────────

type SynergySource struct {
	repo profiles.Repository
}

func NewSynergySource(repo profiles.Repository) *SynergySource {
	return &SynergySource{repo: repo}
}

func (s *SynergySource) Def() domain.FeatureDef {
	return domain.FeatureDef{Name: "mean_syn_with_allies", Dtype: "f32", SourceHash: hashOf("mean_syn: AVG(wr_shrunk) FROM mv_hero_synergy WHERE hero_a IN (allies) AND hero_b=candidate")}
}

func (s *SynergySource) Compute(ctx context.Context, st *domain.DraftState, candidates []domain.HeroID) (map[domain.HeroID]float64, error) {
	result, err := s.repo.SynergyAvgBatch(ctx, st.AllyPicks(), candidates)
	if err != nil {
		return nil, fmt.Errorf("synergy: %w", err)
	}
	return result, nil
}

// ──────────────────────────────────────────────
// CounterSource — average counter WR vs enemy picks
// ──────────────────────────────────────────────

type CounterSource struct {
	repo profiles.Repository
}

func NewCounterSource(repo profiles.Repository) *CounterSource {
	return &CounterSource{repo: repo}
}

func (s *CounterSource) Def() domain.FeatureDef {
	return domain.FeatureDef{Name: "mean_counter_vs_enemies", Dtype: "f32", SourceHash: hashOf("mean_counter: AVG(wr_shrunk) FROM mv_hero_counter WHERE hero_a=candidate AND hero_b IN (enemies)")}
}

func (s *CounterSource) Compute(ctx context.Context, st *domain.DraftState, candidates []domain.HeroID) (map[domain.HeroID]float64, error) {
	result, err := s.repo.CounterAvgBatch(ctx, candidates, st.EnemyPicks())
	if err != nil {
		return nil, fmt.Errorf("counter: %w", err)
	}
	return result, nil
}

// ──────────────────────────────────────────────
// HeroMetaPrimaryAttrSource — encoded primary attribute
// ──────────────────────────────────────────────

type HeroMetaPrimaryAttrSource struct {
	catalog domain.HeroCatalog
}

func NewHeroMetaPrimaryAttrSource(catalog domain.HeroCatalog) *HeroMetaPrimaryAttrSource {
	return &HeroMetaPrimaryAttrSource{catalog: catalog}
}

func (s *HeroMetaPrimaryAttrSource) Def() domain.FeatureDef {
	return domain.FeatureDef{Name: "hero_meta_primary_attr", Dtype: "f32", SourceHash: hashOf("primary_attr: str=1, agi=2, int=3, all=4")}
}

func (s *HeroMetaPrimaryAttrSource) Compute(ctx context.Context, st *domain.DraftState, candidates []domain.HeroID) (map[domain.HeroID]float64, error) {
	meta, err := HeroMetaFeatures(s.catalog, candidates)
	if err != nil {
		return nil, fmt.Errorf("meta primary attr: %w", err)
	}
	result := make(map[domain.HeroID]float64, len(candidates))
	for _, h := range candidates {
		result[h] = meta[h][0]
	}
	return result, nil
}

// ──────────────────────────────────────────────
// HeroMetaRoleCountSource — number of roles for this hero
// ──────────────────────────────────────────────

type HeroMetaRoleCountSource struct {
	catalog domain.HeroCatalog
}

func NewHeroMetaRoleCountSource(catalog domain.HeroCatalog) *HeroMetaRoleCountSource {
	return &HeroMetaRoleCountSource{catalog: catalog}
}

func (s *HeroMetaRoleCountSource) Def() domain.FeatureDef {
	return domain.FeatureDef{Name: "hero_meta_role_count", Dtype: "f32", SourceHash: hashOf("role_count: len(hero.Roles)")}
}

func (s *HeroMetaRoleCountSource) Compute(ctx context.Context, st *domain.DraftState, candidates []domain.HeroID) (map[domain.HeroID]float64, error) {
	meta, err := HeroMetaFeatures(s.catalog, candidates)
	if err != nil {
		return nil, fmt.Errorf("meta role count: %w", err)
	}
	result := make(map[domain.HeroID]float64, len(candidates))
	for _, h := range candidates {
		result[h] = meta[h][1]
	}
	return result, nil
}

// ──────────────────────────────────────────────
// PlayerComfortSource — average player comfort across roster
// ──────────────────────────────────────────────

type PlayerComfortSource struct {
	repo profiles.Repository
}

func NewPlayerComfortSource(repo profiles.Repository) *PlayerComfortSource {
	return &PlayerComfortSource{repo: repo}
}

func (s *PlayerComfortSource) Def() domain.FeatureDef {
	return domain.FeatureDef{Name: "player_comfort", Dtype: "f32", SourceHash: hashOf("player_comfort: wr_shrunk FROM mv_player_hero_profile WHERE account_id=? AND hero_id=?")}
}

func (s *PlayerComfortSource) Compute(ctx context.Context, st *domain.DraftState, candidates []domain.HeroID) (map[domain.HeroID]float64, error) {
	result, err := s.repo.RosterComfortAvgBatch(ctx, st.Roster(), candidates)
	if err != nil {
		return nil, fmt.Errorf("player comfort: %w", err)
	}
	return result, nil
}

// ──────────────────────────────────────────────
// StarThreatSource — opponent's signature hero threat
// ──────────────────────────────────────────────

type StarThreatSource struct {
	repo profiles.Repository
}

func NewStarThreatSource(repo profiles.Repository) *StarThreatSource {
	return &StarThreatSource{repo: repo}
}

func (s *StarThreatSource) Def() domain.FeatureDef {
	return domain.FeatureDef{Name: "star_threat", Dtype: "f32", SourceHash: hashOf("star_threat: opponent signature hero threat level")}
}

func (s *StarThreatSource) Compute(ctx context.Context, st *domain.DraftState, candidates []domain.HeroID) (map[domain.HeroID]float64, error) {
	result, err := s.repo.StarThreatBatch(ctx, st.ThemTeamID(), candidates, 3)
	if err != nil {
		return nil, fmt.Errorf("star threat: %w", err)
	}
	return result, nil
}

// hashOf computes a truncated SHA-256 hex digest of a source description string.
func hashOf(desc string) string {
	h := sha256.Sum256([]byte(desc))
	return fmt.Sprintf("%x", h[:8])
}
