package features

import (
	"context"
	"testing"

	"github.com/user-for-download/dota2-analysis/go-analysis/internal/domain"
)

// mockSource returns a fixed value for every hero.
type mockSource struct {
	name  string
	value float64
}

func (m *mockSource) Def() domain.FeatureDef {
	return domain.FeatureDef{Name: m.name, Dtype: "f32", SourceHash: "mock"}
}

func (m *mockSource) Compute(_ context.Context, _ *domain.DraftState, candidates []domain.HeroID) (map[domain.HeroID]float64, error) {
	out := make(map[domain.HeroID]float64, len(candidates))
	for _, h := range candidates {
		out[h] = m.value
	}
	return out, nil
}

func TestNewBuilder(t *testing.T) {
	sources := []FeatureSource{
		&mockSource{name: "a", value: 1},
		&mockSource{name: "b", value: 2},
	}
	b := NewBuilder(sources)
	spec := b.Spec()

	if spec.Version != FeatureSpecVersion {
		t.Errorf("Version = %q, want %q", spec.Version, FeatureSpecVersion)
	}
	if len(spec.Features) != 2 {
		t.Fatalf("got %d features, want 2", len(spec.Features))
	}
	if spec.Features[0].Name != "a" || spec.Features[1].Name != "b" {
		t.Errorf("feature names mismatch: got %v", spec.Features)
	}
}

func TestBuilderSpec(t *testing.T) {
	sources := []FeatureSource{&mockSource{name: "x", value: 5}}
	b := NewBuilder(sources)
	// Spec should return the same instance each call.
	if b.Spec() != b.Spec() {
		t.Error("Spec() should be stable")
	}
}

func TestBuilderBuild(t *testing.T) {
	sources := []FeatureSource{
		&mockSource{name: "a", value: 10},
		&mockSource{name: "b", value: 20},
	}
	b := NewBuilder(sources)
	st := domain.NewDraftState(
		1, domain.SideUs, &dummyPhaseTable{},
		100, 200,
		[]domain.AccountID{1001, 1002}, []domain.AccountID{2001},
		[]domain.HeroID{1}, []domain.HeroID{2},
		[]domain.HeroID(nil), []domain.HeroID(nil),
		0,
	)
	candidates := []domain.HeroID{3, 4}

	vectors, err := b.Build(context.Background(), st, candidates)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if len(vectors) != 2 {
		t.Fatalf("got %d vectors, want 2", len(vectors))
	}

	for i, v := range vectors {
		if v.Hero() != candidates[i] {
			t.Errorf("vector[%d] Hero() = %d, want %d", i, v.Hero(), candidates[i])
		}
		if v.Len() != 2 {
			t.Errorf("vector[%d] Len() = %d, want 2", i, v.Len())
		}
		vals := v.Values()
		if vals[0] != 10 || vals[1] != 20 {
			t.Errorf("vector[%d] values = %v, want [10 20]", i, vals)
		}
	}

	// Spec should be the builder's spec.
	for _, v := range vectors {
		if v.Spec() != b.Spec() {
			t.Error("vector spec should match builder spec")
		}
	}
}

func TestBuilderBuildEmptyCandidates(t *testing.T) {
	b := NewBuilder([]FeatureSource{&mockSource{name: "a", value: 1}})
	vectors, err := b.Build(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if vectors != nil {
		t.Error("expected nil for empty candidates")
	}
}

// dummyPhaseTable implements domain.PhaseTable for tests.
type dummyPhaseTable struct{}

func (d *dummyPhaseTable) At(slot int) (domain.Phase, bool) {
	return domain.Phase{Name: "test", IsBan: false, ActingTeam: domain.DraftRadiant}, true
}

func (d *dummyPhaseTable) Len() int { return 20 }
