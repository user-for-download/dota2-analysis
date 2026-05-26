package features

import (
	"context"
	"fmt"

	"github.com/user-for-download/dota2-analysis/go-analysis/internal/domain"
	"github.com/user-for-download/dota2-analysis/go-analysis/internal/profiles"
)

// ──────────────────────────────────────────────
// AttrIsStrSource — 1.0 for STR or universal heroes, 0.0 otherwise
// ──────────────────────────────────────────────

type AttrIsStrSource struct {
	catalog domain.HeroCatalog
}

func NewAttrIsStrSource(catalog domain.HeroCatalog) *AttrIsStrSource {
	return &AttrIsStrSource{catalog: catalog}
}

func (s *AttrIsStrSource) Def() domain.FeatureDef {
	return domain.FeatureDef{
		Name:       "attr_is_str",
		Dtype:      "f32",
		SourceHash: hashOf("attr_is_str: primary_attr==str or all"),
	}
}

func (s *AttrIsStrSource) Compute(ctx context.Context, st *domain.DraftState, candidates []domain.HeroID) (map[domain.HeroID]float64, error) {
	meta, err := HeroMetaFeatures(s.catalog, candidates)
	if err != nil {
		return nil, fmt.Errorf("attr_is_str: %w", err)
	}
	result := make(map[domain.HeroID]float64, len(candidates))
	for _, h := range candidates {
		attr := meta[h][0]
		if attr == 1 || attr == 4 { // str or universal
			result[h] = 1.0
		}
	}
	return result, nil
}

// ──────────────────────────────────────────────
// AttrIsAgiSource — 1.0 for AGI or universal heroes, 0.0 otherwise
// ──────────────────────────────────────────────

type AttrIsAgiSource struct {
	catalog domain.HeroCatalog
}

func NewAttrIsAgiSource(catalog domain.HeroCatalog) *AttrIsAgiSource {
	return &AttrIsAgiSource{catalog: catalog}
}

func (s *AttrIsAgiSource) Def() domain.FeatureDef {
	return domain.FeatureDef{
		Name:       "attr_is_agi",
		Dtype:      "f32",
		SourceHash: hashOf("attr_is_agi: primary_attr==agi or all"),
	}
}

func (s *AttrIsAgiSource) Compute(ctx context.Context, st *domain.DraftState, candidates []domain.HeroID) (map[domain.HeroID]float64, error) {
	meta, err := HeroMetaFeatures(s.catalog, candidates)
	if err != nil {
		return nil, fmt.Errorf("attr_is_agi: %w", err)
	}
	result := make(map[domain.HeroID]float64, len(candidates))
	for _, h := range candidates {
		attr := meta[h][0]
		if attr == 2 || attr == 4 { // agi or universal
			result[h] = 1.0
		}
	}
	return result, nil
}

// ──────────────────────────────────────────────
// AttrIsIntSource — 1.0 for INT or universal heroes, 0.0 otherwise
// ──────────────────────────────────────────────

type AttrIsIntSource struct {
	catalog domain.HeroCatalog
}

func NewAttrIsIntSource(catalog domain.HeroCatalog) *AttrIsIntSource {
	return &AttrIsIntSource{catalog: catalog}
}

func (s *AttrIsIntSource) Def() domain.FeatureDef {
	return domain.FeatureDef{
		Name:       "attr_is_int",
		Dtype:      "f32",
		SourceHash: hashOf("attr_is_int: primary_attr==int or all"),
	}
}

func (s *AttrIsIntSource) Compute(ctx context.Context, st *domain.DraftState, candidates []domain.HeroID) (map[domain.HeroID]float64, error) {
	meta, err := HeroMetaFeatures(s.catalog, candidates)
	if err != nil {
		return nil, fmt.Errorf("attr_is_int: %w", err)
	}
	result := make(map[domain.HeroID]float64, len(candidates))
	for _, h := range candidates {
		attr := meta[h][0]
		if attr == 3 || attr == 4 { // int or universal
			result[h] = 1.0
		}
	}
	return result, nil
}

// ──────────────────────────────────────────────
// AttrFitScoreSource — team_picks * (int*0.5 + agi*0.3 + str*0.2)
// ──────────────────────────────────────────────

type AttrFitScoreSource struct {
	repo    profiles.Repository
	catalog domain.HeroCatalog
}

func NewAttrFitScoreSource(repo profiles.Repository, catalog domain.HeroCatalog) *AttrFitScoreSource {
	return &AttrFitScoreSource{repo: repo, catalog: catalog}
}

func (s *AttrFitScoreSource) Def() domain.FeatureDef {
	return domain.FeatureDef{
		Name:       "attr_fit_score",
		Dtype:      "f32",
		SourceHash: hashOf("attr_fit_score: team_picks * (is_int*0.5 + is_agi*0.3 + is_str*0.2)"),
	}
}

func (s *AttrFitScoreSource) Compute(ctx context.Context, st *domain.DraftState, candidates []domain.HeroID) (map[domain.HeroID]float64, error) {
	meta, err := HeroMetaFeatures(s.catalog, candidates)
	if err != nil {
		return nil, fmt.Errorf("attr_fit_score meta: %w", err)
	}
	teamPicks, err := s.repo.TeamHeroStatsBatch(ctx, st.TeamID(), candidates)
	if err != nil {
		return nil, fmt.Errorf("attr_fit_score team_picks: %w", err)
	}
	result := make(map[domain.HeroID]float64, len(candidates))
	for _, h := range candidates {
		attr := meta[h][0]
		tp := float64(teamPicks[h].Games)
		// Attribute weights: INT=0.5, AGI=0.3, STR=0.2.
		// Universal heroes (attr==4) get the MAX weight (0.5) rather
		// than the sum of all three — prevents a 3x bias.
		var attrW float64
		switch {
		case attr == 3 || attr == 4: // int or universal → max(0.5)
			attrW = 0.5
		case attr == 2: // agi → 0.3
			attrW = 0.3
		default: // str or unknown → 0.2
			attrW = 0.2
		}
		result[h] = tp * attrW
	}
	return result, nil
}
