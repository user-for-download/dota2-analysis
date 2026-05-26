package features

import (
	"context"
	"fmt"
	"math"

	"github.com/user-for-download/dota2-analysis/go-analysis/internal/domain"
	"github.com/user-for-download/dota2-analysis/go-analysis/internal/profiles"
)

// ──────────────────────────────────────────────
// HeroPickRateSource — shrunk global pick rate per hero
// ──────────────────────────────────────────────

type HeroPickRateSource struct {
	repo profiles.Repository
}

func NewHeroPickRateSource(repo profiles.Repository) *HeroPickRateSource {
	return &HeroPickRateSource{repo: repo}
}

func (s *HeroPickRateSource) Def() domain.FeatureDef {
	return domain.FeatureDef{
		Name:       "hero_pick_rate",
		Dtype:      "f32",
		SourceHash: hashOf("hero_pick_rate: shrunk pick freq from full corpus"),
	}
}

func (s *HeroPickRateSource) Compute(ctx context.Context, st *domain.DraftState, candidates []domain.HeroID) (map[domain.HeroID]float64, error) {
	stats, err := s.repo.GlobalHeroStatsBatch(ctx, candidates, st.Patch())
	if err != nil {
		return nil, fmt.Errorf("hero pick rate: %w", err)
	}
	// Use a constant baseline for the pick-rate denominator.
	// The exact value doesn't need to match training — the relative
	// ordering among heroes is what provides ranking signal.
	const nDecisions = 20000.0
	result := make(map[domain.HeroID]float64, len(candidates))
	for _, h := range candidates {
		gs := stats[h]
		result[h] = (float64(gs.PickCount) + 2.0) / (nDecisions + 4.0)
	}
	return result, nil
}

// ──────────────────────────────────────────────
// HeroWRSource — shrunk global win rate per hero
// ──────────────────────────────────────────────

type HeroWRSource struct {
	repo profiles.Repository
}

func NewHeroWRSource(repo profiles.Repository) *HeroWRSource {
	return &HeroWRSource{repo: repo}
}

func (s *HeroWRSource) Def() domain.FeatureDef {
	return domain.FeatureDef{
		Name:       "hero_wr",
		Dtype:      "f32",
		SourceHash: hashOf("hero_wr: shrunk win rate from full corpus"),
	}
}

func (s *HeroWRSource) Compute(ctx context.Context, st *domain.DraftState, candidates []domain.HeroID) (map[domain.HeroID]float64, error) {
	stats, err := s.repo.GlobalHeroStatsBatch(ctx, candidates, st.Patch())
	if err != nil {
		return nil, fmt.Errorf("hero wr: %w", err)
	}
	result := make(map[domain.HeroID]float64, len(candidates))
	for _, h := range candidates {
		gs := stats[h]
		result[h] = (float64(gs.WinCount) + 10.0) / (float64(gs.PickCount) + 20.0)
	}
	return result, nil
}

// ──────────────────────────────────────────────
// HeroPopularitySource — log1p(pick_count) per hero
// ──────────────────────────────────────────────

type HeroPopularitySource struct {
	repo profiles.Repository
}

func NewHeroPopularitySource(repo profiles.Repository) *HeroPopularitySource {
	return &HeroPopularitySource{repo: repo}
}

func (s *HeroPopularitySource) Def() domain.FeatureDef {
	return domain.FeatureDef{
		Name:       "hero_popularity",
		Dtype:      "f32",
		SourceHash: hashOf("hero_popularity: log1p(pick_count)"),
	}
}

func (s *HeroPopularitySource) Compute(ctx context.Context, st *domain.DraftState, candidates []domain.HeroID) (map[domain.HeroID]float64, error) {
	stats, err := s.repo.GlobalHeroStatsBatch(ctx, candidates, st.Patch())
	if err != nil {
		return nil, fmt.Errorf("hero popularity: %w", err)
	}
	result := make(map[domain.HeroID]float64, len(candidates))
	for _, h := range candidates {
		gs := stats[h]
		result[h] = math.Log1p(float64(gs.PickCount))
	}
	return result, nil
}
