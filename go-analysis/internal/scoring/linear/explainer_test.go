package linear

import (
	"context"
	"testing"

	"github.com/user-for-download/dota2-analysis/go-analysis/internal/domain"
)

func TestExplain(t *testing.T) {
	// Use only features we explicitly weight to isolate the test.
	spec := mustSpec(t, []domain.FeatureDef{
		{Name: "strong", Dtype: "f32"},
		{Name: "weak", Dtype: "f32"},
		{Name: "risky", Dtype: "f32"},
		{Name: "neutral", Dtype: "f32"},
	})
	s := NewScorer(spec)
	// Zero all default weights first, then set only the ones we test.
	for name := range s.Weights() {
		s.SetWeight(name, 0)
	}
	s.SetWeight("strong", 0.30)  // > 0.1 → reason
	s.SetWeight("weak", 0.05)    // ≤ 0.1 → not a reason
	s.SetWeight("risky", -0.30)  // < -0.05 → risk
	s.SetWeight("neutral", 0.0)  // between -0.05 and 0.1 → nothing

	e := NewExplainer(s)
	reasons, risks, err := e.Explain(context.Background(), domain.HeroID(1), 0.8)
	if err != nil {
		t.Fatalf("Explain failed: %v", err)
	}

	if len(reasons) != 1 {
		t.Errorf("got %d reasons, want 1", len(reasons))
	} else if reasons[0].Factor != "strong" {
		t.Errorf("reason factor = %q, want %q", reasons[0].Factor, "strong")
	}

	if len(risks) != 1 {
		t.Errorf("got %d risks, want 1", len(risks))
	} else if risks[0].Factor != "risky" {
		t.Errorf("risk factor = %q, want %q", risks[0].Factor, "risky")
	}

	// Verify delta values.
	if reasons[0].Delta != 0.30 {
		t.Errorf("reason delta = %f, want 0.30", reasons[0].Delta)
	}
	if risks[0].Delta != -0.30 {
		t.Errorf("risk delta = %f, want -0.30", risks[0].Delta)
	}
}

func TestExplainBoundary(t *testing.T) {
	spec := mustSpec(t, []domain.FeatureDef{
		{Name: "borderline_positive", Dtype: "f32"},
		{Name: "borderline_negative", Dtype: "f32"},
	})
	s := NewScorer(spec)
	// Zero defaults so only our test weights matter.
	for name := range s.Weights() {
		s.SetWeight(name, 0)
	}
	s.SetWeight("borderline_positive", 0.10) // exactly 0.1 → not a reason (> threshold)
	s.SetWeight("borderline_negative", -0.05) // exactly -0.05 → not a risk (< threshold)

	e := NewExplainer(s)
	reasons, risks, err := e.Explain(context.Background(), domain.HeroID(1), 0.5)
	if err != nil {
		t.Fatalf("Explain failed: %v", err)
	}
	if len(reasons) != 0 {
		t.Errorf("expected 0 reasons for weight 0.10, got %d", len(reasons))
	}
	if len(risks) != 0 {
		t.Errorf("expected 0 risks for weight -0.05, got %d", len(risks))
	}
}

func TestExplainNoWeights(t *testing.T) {
	// Scorer with no SetWeight — uses defaults. Verify no crash.
	spec := mustSpec(t, []domain.FeatureDef{
		{Name: "team_picks", Dtype: "f32"},
		{Name: "team_wr_shrunk", Dtype: "f32"},
	})
	s := NewScorer(spec)
	e := NewExplainer(s)
	reasons, risks, err := e.Explain(context.Background(), domain.HeroID(1), 0.5)
	if err != nil {
		t.Fatalf("Explain failed: %v", err)
	}
	// At least team_wr_shrunk (0.30) should be a reason.
	found := false
	for _, r := range reasons {
		if r.Factor == "team_wr_shrunk" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected team_wr_shrunk in reasons, got %v", reasons)
	}
	_ = risks
}
