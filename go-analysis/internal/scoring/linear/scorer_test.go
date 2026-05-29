package linear

import (
	"context"
	"testing"

	"github.com/user-for-download/dota2-analysis/go-analysis/internal/domain"
)

func mustSpec(t *testing.T, defs []domain.FeatureDef) *domain.FeatureSpec {
	t.Helper()
	s := &domain.FeatureSpec{Version: "test-v1", Features: defs}
	s.MustValidate()
	return s
}

func TestScore(t *testing.T) {
	spec := mustSpec(t, []domain.FeatureDef{
		{Name: "a", Dtype: "f32"},
		{Name: "b", Dtype: "f32"},
	})

	// Scorer with subset of default weights replaced for deterministic test.
	s := NewScorer(spec)
	s.SetWeight("a", 0.3)
	s.SetWeight("b", 0.7)

	vectors := []*domain.FeatureVector{
		domain.NewFeatureVector(spec, domain.HeroID(1), []float64{1.0, 0.0}), // 0.5 + 0.3*1 + 0.7*0 = 0.8
		domain.NewFeatureVector(spec, domain.HeroID(2), []float64{0.0, 1.0}), // 0.5 + 0.3*0 + 0.7*1 = 1.2
		domain.NewFeatureVector(spec, domain.HeroID(3), []float64{0.5, 0.5}), // 0.5 + 0.3*0.5 + 0.7*0.5 = 1.0
	}

	scores, err := s.Score(context.Background(), vectors)
	if err != nil {
		t.Fatalf("Score failed: %v", err)
	}
	if len(scores) != 3 {
		t.Fatalf("got %d scores, want 3", len(scores))
	}

	expected := []struct {
		hero  domain.HeroID
		value float64
	}{
		{domain.HeroID(1), 0.8},
		{domain.HeroID(2), 1.2},
		{domain.HeroID(3), 1.0},
	}

	for i, e := range expected {
		if scores[i].Hero != e.hero {
			t.Errorf("scores[%d] Hero = %d, want %d", i, scores[i].Hero, e.hero)
		}
		if scores[i].Value != e.value {
			t.Errorf("scores[%d] Value = %f, want %f", i, scores[i].Value, e.value)
		}
	}
}

func TestScoreEmpty(t *testing.T) {
	spec := mustSpec(t, []domain.FeatureDef{{Name: "a", Dtype: "f32"}})
	s := NewScorer(spec)
	scores, err := s.Score(context.Background(), nil)
	if err != nil {
		t.Fatalf("Score failed: %v", err)
	}
	if len(scores) != 0 {
		t.Errorf("expected 0 scores, got %d", len(scores))
	}
}

func TestVersion(t *testing.T) {
	spec := mustSpec(t, []domain.FeatureDef{{Name: "a", Dtype: "f32"}})
	s := NewScorer(spec)
	if v := s.Version(); v != "linear-v1" {
		t.Errorf("Version = %q, want %q", v, "linear-v1")
	}
}

func TestWeights(t *testing.T) {
	spec := mustSpec(t, []domain.FeatureDef{{Name: "a", Dtype: "f32"}})
	s := NewScorer(spec)
	s.SetWeight("a", 0.42)

	// Weights() must return a copy.
	w1 := s.Weights()
	w1["a"] = 999

	w2 := s.Weights()
	if w2["a"] != 0.42 {
		t.Errorf("weights were mutated: got %f, want 0.42", w2["a"])
	}
}

func TestDefaultWeights(t *testing.T) {
	spec := mustSpec(t, []domain.FeatureDef{
		{Name: "team_picks", Dtype: "f32"},
		{Name: "team_wr_shrunk", Dtype: "f32"},
		{Name: "mean_syn_with_allies", Dtype: "f32"},
		{Name: "mean_counter_vs_enemies", Dtype: "f32"},
		{Name: "hero_meta_primary_attr", Dtype: "f32"},
		{Name: "hero_meta_role_count", Dtype: "f32"},
		{Name: "player_comfort", Dtype: "f32"},
		{Name: "star_threat", Dtype: "f32"},
	})
	s := NewScorer(spec)
	w := s.Weights()
	if len(w) != 24 {
		t.Errorf("got %d weights, want 24 (all features must have non-zero weights)", len(w))
	}
	// Spot-check known values across categories.
	if w["team_wr_shrunk"] != 0.30 {
		t.Errorf("team_wr_shrunk = %f, want 0.30", w["team_wr_shrunk"])
	}
	if w["star_threat"] != -0.10 {
		t.Errorf("star_threat = %f, want -0.10", w["star_threat"])
	}
	if w["hero_wr"] != 0.12 {
		t.Errorf("hero_wr = %f, want 0.12", w["hero_wr"])
	}
	if w["hero_pick_rate"] != 0.08 {
		t.Errorf("hero_pick_rate = %f, want 0.08", w["hero_pick_rate"])
	}
	if w["attr_fit_score"] != 0.04 {
		t.Errorf("attr_fit_score = %f, want 0.04", w["attr_fit_score"])
	}
	// No weight should be zero (silent discard detection).
	for name, weight := range w {
		if weight == 0 {
			t.Errorf("feature %q has zero weight — contributes nothing", name)
		}
	}
}
