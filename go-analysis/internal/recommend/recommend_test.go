package recommend

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/user-for-download/dota2-analysis/go-analysis/internal/domain"
)

// ─── Mocks ──────────────────────────────────────────────────

type mockBuilder struct {
	specFn   func() *domain.FeatureSpec
	buildFn  func(ctx context.Context, st *domain.DraftState, candidates []domain.HeroID) ([]*domain.FeatureVector, error)
}

func (m *mockBuilder) Spec() *domain.FeatureSpec                                               { return m.specFn() }
func (m *mockBuilder) Build(ctx context.Context, st *domain.DraftState, candidates []domain.HeroID) ([]*domain.FeatureVector, error) {
	return m.buildFn(ctx, st, candidates)
}

type mockScorer struct {
	specFn    func() *domain.FeatureSpec
	versionFn func() string
	scoreFn   func(ctx context.Context, vectors []*domain.FeatureVector) ([]domain.Score, error)
}

func (m *mockScorer) Spec() *domain.FeatureSpec                       { return m.specFn() }
func (m *mockScorer) Version() string                                 { return m.versionFn() }
func (m *mockScorer) Score(ctx context.Context, vectors []*domain.FeatureVector) ([]domain.Score, error) {
	return m.scoreFn(ctx, vectors)
}

type mockExplainer struct {
	explainFn func(ctx context.Context, vector *domain.FeatureVector, score float64) ([]domain.Reason, []domain.Reason, error)
}

func (m *mockExplainer) Explain(ctx context.Context, vector *domain.FeatureVector, score float64) ([]domain.Reason, []domain.Reason, error) {
	return m.explainFn(ctx, vector, score)
}

type mockCatalog struct{}

func (mockCatalog) Name(id domain.HeroID) string {
	names := map[domain.HeroID]string{1: "npc_dota_hero_axe", 2: "npc_dota_hero_pudge", 3: "npc_dota_hero_puck"}
	if n, ok := names[id]; ok {
		return n
	}
	return "unknown"
}
func (mockCatalog) Info(id domain.HeroID) (domain.HeroInfo, bool) {
	return domain.HeroInfo{ID: id, Name: mockCatalog{}.Name(id)}, true
}
func (mockCatalog) Roles(domain.HeroID) []domain.Role { return nil }
func (mockCatalog) All() []domain.HeroID              { return []domain.HeroID{1, 2, 3} }
func (mockCatalog) EachHero(f func(domain.HeroID) bool) {
	for _, id := range []domain.HeroID{1, 2, 3} {
		if !f(id) {
			return
		}
	}
}

func makeDraftState() *domain.DraftState {
	return domain.NewDraftState(
		42, domain.SideUs, domain.CMPhaseTable(),
		100, 200,
		[]domain.AccountID{1001, 1002}, []domain.AccountID{2001, 2002},
		nil, nil, nil, nil,
		0, // slot 0 = first ban phase, radiant acts
	)
}

func makeFeatureVector(heroID domain.HeroID, vals ...float64) *domain.FeatureVector {
	spec := &domain.FeatureSpec{Version: "1", Features: []domain.FeatureDef{{Name: "test", Dtype: "float64"}}}
	return domain.NewFeatureVector(spec, heroID, vals)
}

// ─── Tests ──────────────────────────────────────────────────

func TestRecommend_FullPipeline(t *testing.T) {
	builder := &mockBuilder{
		buildFn: func(ctx context.Context, st *domain.DraftState, candidates []domain.HeroID) ([]*domain.FeatureVector, error) {
			vecs := make([]*domain.FeatureVector, len(candidates))
			for i, h := range candidates {
				vecs[i] = makeFeatureVector(h, 0.5)
			}
			return vecs, nil
		},
	}
	scorer := &mockScorer{
		scoreFn: func(ctx context.Context, vectors []*domain.FeatureVector) ([]domain.Score, error) {
			scores := make([]domain.Score, len(vectors))
			for i, v := range vectors {
				scores[i] = domain.Score{Hero: v.Hero(), Value: float64(v.Hero()) * 0.1}
			}
			return scores, nil
		},
	}
	explainer := &mockExplainer{
		explainFn: func(ctx context.Context, vector *domain.FeatureVector, score float64) ([]domain.Reason, []domain.Reason, error) {
			return []domain.Reason{{Factor: "synergy", Note: "strong", Delta: 0.05}}, nil, nil
		},
	}

	svc := NewService(builder, scorer, explainer, mockCatalog{}).WithLog(slog.Default())
	st := makeDraftState()

	result, err := svc.Recommend(context.Background(), st, 3)
	if err != nil {
		t.Fatalf("Recommend: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Recommendations[0].Score <= result.Recommendations[1].Score {
		t.Error("expected first recommendation to have highest score")
	}
	if len(result.Recommendations) > 3 {
		t.Errorf("expected at most 3 recommendations, got %d", len(result.Recommendations))
	}
	if result.UsedValueModel {
		t.Error("expected UsedValueModel=false (no value scorer)")
	}
}

func TestRecommend_WithValueModel(t *testing.T) {
	builder := &mockBuilder{
		buildFn: func(ctx context.Context, st *domain.DraftState, candidates []domain.HeroID) ([]*domain.FeatureVector, error) {
			vecs := make([]*domain.FeatureVector, len(candidates))
			for i, h := range candidates {
				vecs[i] = makeFeatureVector(h, 0.5)
			}
			return vecs, nil
		},
	}
	imitation := &mockScorer{
		scoreFn: func(ctx context.Context, vectors []*domain.FeatureVector) ([]domain.Score, error) {
			scores := make([]domain.Score, len(vectors))
			for i, v := range vectors {
				scores[i] = domain.Score{Hero: v.Hero(), Value: 0.4 + float64(i)*0.05}
			}
			return scores, nil
		},
	}
	value := &mockScorer{
		scoreFn: func(ctx context.Context, vectors []*domain.FeatureVector) ([]domain.Score, error) {
			scores := make([]domain.Score, len(vectors))
			for i, v := range vectors {
				scores[i] = domain.Score{Hero: v.Hero(), Value: 0.3 + float64(i)*0.1}
			}
			return scores, nil
		},
	}
	explainer := &mockExplainer{
		explainFn: func(ctx context.Context, vector *domain.FeatureVector, score float64) ([]domain.Reason, []domain.Reason, error) {
			return nil, nil, nil
		},
	}

	svc := NewService(builder, imitation, explainer, mockCatalog{}).WithValueScorer(value).WithLog(slog.Default())
	st := makeDraftState()

	result, err := svc.Recommend(context.Background(), st, 3)
	if err != nil {
		t.Fatalf("Recommend: %v", err)
	}
	if !result.UsedValueModel {
		t.Error("expected UsedValueModel=true")
	}
	if len(result.Warnings) != 0 {
		t.Errorf("expected no warnings, got %v", result.Warnings)
	}
}

func TestRecommend_ValueScorerFallback(t *testing.T) {
	builder := &mockBuilder{
		buildFn: func(ctx context.Context, st *domain.DraftState, candidates []domain.HeroID) ([]*domain.FeatureVector, error) {
			vecs := make([]*domain.FeatureVector, len(candidates))
			for i, h := range candidates {
				vecs[i] = makeFeatureVector(h, 0.5)
			}
			return vecs, nil
		},
	}
	imitation := &mockScorer{
		scoreFn: func(ctx context.Context, vectors []*domain.FeatureVector) ([]domain.Score, error) {
			scores := make([]domain.Score, len(vectors))
			for i, v := range vectors {
				scores[i] = domain.Score{Hero: v.Hero(), Value: 0.5}
			}
			return scores, nil
		},
	}
	value := &mockScorer{
		scoreFn: func(ctx context.Context, vectors []*domain.FeatureVector) ([]domain.Score, error) {
			return nil, errors.New("model not loaded")
		},
	}
	explainer := &mockExplainer{
		explainFn: func(ctx context.Context, vector *domain.FeatureVector, score float64) ([]domain.Reason, []domain.Reason, error) {
			return nil, nil, nil
		},
	}

	svc := NewService(builder, imitation, explainer, mockCatalog{}).WithValueScorer(value).WithLog(slog.Default())
	st := makeDraftState()

	result, err := svc.Recommend(context.Background(), st, 3)
	if err != nil {
		t.Fatalf("Recommend: %v", err)
	}
	if !result.UsedValueModel {
		t.Error("expected UsedValueModel=true (we configured one, it just failed)")
	}
	if len(result.Warnings) == 0 {
		t.Error("expected warning about value model failure")
	}
	if len(result.Recommendations) == 0 {
		t.Error("expected recommendations despite value model failure")
	}
}

func TestRecommend_NoCandidates(t *testing.T) {
	// A draft where all heroes are already picked/banned.
	allHeroes := []domain.HeroID{1, 2, 3}
	svc := NewService(&mockBuilder{}, &mockScorer{}, &mockExplainer{}, mockCatalog{}).WithLog(slog.Default())
	st := domain.NewDraftState(
		42, domain.SideUs, domain.CMPhaseTable(),
		100, 200,
		[]domain.AccountID{1001}, []domain.AccountID{2001},
		allHeroes, nil, allHeroes, nil, // all 3 picked + all 3 banned = nothing left
		0,
	)
	result, err := svc.Recommend(context.Background(), st, 5)
	if err != nil {
		t.Fatalf("Recommend: %v", err)
	}
	if len(result.Recommendations) != 0 {
		t.Errorf("expected 0 recommendations, got %d", len(result.Recommendations))
	}
}

func TestRecommend_BuilderError(t *testing.T) {
	builder := &mockBuilder{
		buildFn: func(ctx context.Context, st *domain.DraftState, candidates []domain.HeroID) ([]*domain.FeatureVector, error) {
			return nil, errors.New("builder error")
		},
	}
	svc := NewService(builder, &mockScorer{}, &mockExplainer{}, mockCatalog{}).WithLog(slog.Default())
	_, err := svc.Recommend(context.Background(), makeDraftState(), 3)
	if err == nil {
		t.Fatal("expected error from builder")
	}
}

func TestRecommend_ScorerError(t *testing.T) {
	builder := &mockBuilder{
		buildFn: func(ctx context.Context, st *domain.DraftState, candidates []domain.HeroID) ([]*domain.FeatureVector, error) {
			vecs := make([]*domain.FeatureVector, len(candidates))
			for i, h := range candidates {
				vecs[i] = makeFeatureVector(h, 0.5)
			}
			return vecs, nil
		},
	}
	scorer := &mockScorer{
		scoreFn: func(ctx context.Context, vectors []*domain.FeatureVector) ([]domain.Score, error) {
			return nil, errors.New("scorer error")
		},
	}
	svc := NewService(builder, scorer, &mockExplainer{}, mockCatalog{}).WithLog(slog.Default())
	_, err := svc.Recommend(context.Background(), makeDraftState(), 3)
	if err == nil {
		t.Fatal("expected error from scorer")
	}
}

func TestRecommend_TopK(t *testing.T) {
	builder := &mockBuilder{
		buildFn: func(ctx context.Context, st *domain.DraftState, candidates []domain.HeroID) ([]*domain.FeatureVector, error) {
			vecs := make([]*domain.FeatureVector, len(candidates))
			for i, h := range candidates {
				vecs[i] = makeFeatureVector(h, 0.5)
			}
			return vecs, nil
		},
	}
	scorer := &mockScorer{
		scoreFn: func(ctx context.Context, vectors []*domain.FeatureVector) ([]domain.Score, error) {
			scores := make([]domain.Score, len(vectors))
			for i, v := range vectors {
				scores[i] = domain.Score{Hero: v.Hero(), Value: float64(v.Hero())}
			}
			return scores, nil
		},
	}
	explainer := &mockExplainer{
		explainFn: func(ctx context.Context, vector *domain.FeatureVector, score float64) ([]domain.Reason, []domain.Reason, error) {
			return nil, nil, nil
		},
	}
	svc := NewService(builder, scorer, explainer, mockCatalog{}).WithLog(slog.Default())

	// Request top-1
	result, err := svc.Recommend(context.Background(), makeDraftState(), 1)
	if err != nil {
		t.Fatalf("Recommend: %v", err)
	}
	if len(result.Recommendations) != 1 {
		t.Errorf("expected 1 recommendation, got %d", len(result.Recommendations))
	}
}

// ─── Ensemble Tests ────────────────────────────────────────

func TestEnsemble_Default(t *testing.T) {
	e := DefaultEnsemble()
	imitation := []domain.Score{{Hero: 1, Value: 0.8}, {Hero: 2, Value: 0.6}}
	result := e.Combine(imitation, nil)
	if len(result) != 2 {
		t.Fatalf("expected 2 scores, got %d", len(result))
	}
	if result[0].Value != 0.8 || result[1].Value != 0.6 {
		t.Error("default ensemble should pass through imitation scores unchanged")
	}
}

func TestEnsemble_Balanced(t *testing.T) {
	e := BalancedEnsemble()
	imitation := []domain.Score{{Hero: 1, Value: 0.8}, {Hero: 2, Value: 0.6}}
	value := []domain.Score{{Hero: 1, Value: 0.5}, {Hero: 2, Value: 0.9}}
	result := e.Combine(imitation, value)
	if len(result) != 2 {
		t.Fatalf("expected 2 scores, got %d", len(result))
	}
	// Hero 1: 0.7*0.8 + 0.3*0.5 = 0.56 + 0.15 = 0.71
	expected1 := 0.7*0.8 + 0.3*0.5
	if result[0].Value != expected1 {
		t.Errorf("hero 1: expected %f, got %f", expected1, result[0].Value)
	}
	// Hero 2: 0.7*0.6 + 0.3*0.9 = 0.42 + 0.27 = 0.69
	expected2 := 0.7*0.6 + 0.3*0.9
	if result[1].Value != expected2 {
		t.Errorf("hero 2: expected %f, got %f", expected2, result[1].Value)
	}
}

func TestEnsemble_Balanced_MissingValue(t *testing.T) {
	e := BalancedEnsemble()
	imitation := []domain.Score{{Hero: 1, Value: 0.8}, {Hero: 2, Value: 0.6}}
	value := []domain.Score{{Hero: 1, Value: 0.5}} // hero 2 missing from value
	result := e.Combine(imitation, value)
	if len(result) != 2 {
		t.Fatalf("expected 2 scores, got %d", len(result))
	}
	// Hero 1: 0.7*0.8 + 0.3*0.5 = 0.71
	expected1 := 0.7*0.8 + 0.3*0.5
	if result[0].Value != expected1 {
		t.Errorf("hero 1: expected %f, got %f", expected1, result[0].Value)
	}
	// Hero 2: no value score → use imitation score: 0.7*0.6 + 0.3*0.6 = 0.6
	if result[1].Value != 0.6 {
		t.Errorf("hero 2 (missing value): expected 0.6, got %f", result[1].Value)
	}
}

// ─── CandidateGen Tests ────────────────────────────────────

func TestCandidateGen_Generate(t *testing.T) {
	g := &CandidateGen{Catalog: mockCatalog{}}
	st := domain.NewDraftState(
		42, domain.SideUs, domain.CMPhaseTable(),
		100, 200,
		nil, nil,
		[]domain.HeroID{1}, nil, // hero 1 picked by us
		nil, nil,
		0,
	)
	candidates := g.Generate(st)
	// Out of [1,2,3], hero 1 is picked, so candidates should be [2,3]
	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d: %v", len(candidates), candidates)
	}
	for _, h := range candidates {
		if h == 1 {
			t.Error("candidate 1 should not appear (already picked)")
		}
	}
}

// ─── Reasons Tests ─────────────────────────────────────────

func TestFormatReasons(t *testing.T) {
	rec := domain.Recommendation{
		Reasons: []domain.Reason{
			{Factor: "synergy", Note: "strong", Delta: 0.05},
		},
	}
	out := FormatReasons(rec)
	if out != "✓ synergy (strong)" {
		t.Errorf("unexpected format: %q", out)
	}
}

func TestFormatReasons_Empty(t *testing.T) {
	out := FormatReasons(domain.Recommendation{})
	if out != "No specific reasons available" {
		t.Errorf("unexpected empty format: %q", out)
	}
}

func TestFormatRisks(t *testing.T) {
	rec := domain.Recommendation{Risks: []string{"counter pick"}}
	out := FormatRisks(rec)
	if out != "⚠ counter pick" {
		t.Errorf("unexpected risk format: %q", out)
	}
}

func TestFormatRisks_Empty(t *testing.T) {
	out := FormatRisks(domain.Recommendation{})
	if out != "" {
		t.Errorf("expected empty, got %q", out)
	}
}

func TestReasonsFromRiskReasons(t *testing.T) {
	rs := []domain.Reason{
		{Factor: "counter", Note: "puck hard counters axe", Delta: -0.1},
	}
	out := reasonsFromRiskReasons(rs)
	if len(out) != 1 {
		t.Fatalf("expected 1 string, got %d", len(out))
	}
	expected := "counter: puck hard counters axe"
	if out[0] != expected {
		t.Errorf("expected %q, got %q", expected, out[0])
	}
}

func TestReasonsFromRiskReasons_Empty(t *testing.T) {
	out := reasonsFromRiskReasons(nil)
	if out != nil {
		t.Errorf("expected nil, got %v", out)
	}
}
