package lgbm

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/user-for-download/dota2-analysis/go-analysis/internal/domain"
)

// ──────────────────────────────────────────────
// mockModel
// ──────────────────────────────────────────────

type mockModel struct {
	predictFn func(values []float64, nRows, nCols int, out []float64, startIter, numIter int) error
}

func (m *mockModel) PredictDense(values []float64, nRows, nCols int, out []float64, startIter, numIter int) error {
	return m.predictFn(values, nRows, nCols, out, startIter, numIter)
}

// ──────────────────────────────────────────────
// helpers
// ──────────────────────────────────────────────

func testSpec() *domain.FeatureSpec {
	return &domain.FeatureSpec{
		Version: "test-v1",
		Features: []domain.FeatureDef{
			{Name: "win_rate", Dtype: "float"},
			{Name: "pick_rate", Dtype: "float"},
		},
	}
}

func testVector(spec *domain.FeatureSpec, hero domain.HeroID, values []float64) *domain.FeatureVector {
	return domain.NewFeatureVector(spec, hero, values)
}

// ──────────────────────────────────────────────
// Scorer.Score
// ──────────────────────────────────────────────

func TestScorerScore_EmptyVectors(t *testing.T) {
	s := &Scorer{ens: new(mockModel), spec: testSpec()}
	scores, err := s.Score(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected nil error for empty vectors, got %v", err)
	}
	if scores != nil {
		t.Fatalf("expected nil scores for empty vectors, got %v", scores)
	}

	scores, err = s.Score(context.Background(), []*domain.FeatureVector{})
	if err != nil {
		t.Fatalf("expected nil error for empty slice, got %v", err)
	}
	if scores != nil {
		t.Fatalf("expected nil scores for empty slice, got %v", scores)
	}
}

func TestScorerScore_SpecMismatch_nilSpec(t *testing.T) {
	s := &Scorer{ens: new(mockModel), spec: nil}
	v := testVector(testSpec(), 1, []float64{0.5, 0.3})
	_, err := s.Score(context.Background(), []*domain.FeatureVector{v})
	if err == nil {
		t.Fatal("expected error for nil scorer spec")
	}
}

func TestScorerScore_SpecMismatch_differentSpec(t *testing.T) {
	scorerSpec := testSpec()
	s := &Scorer{ens: new(mockModel), spec: scorerSpec}

	otherSpec := &domain.FeatureSpec{Version: "other-v1", Features: []domain.FeatureDef{{Name: "x", Dtype: "float"}}}
	v := testVector(otherSpec, 1, []float64{0.5})
	_, err := s.Score(context.Background(), []*domain.FeatureVector{v})
	if err == nil {
		t.Fatal("expected error for spec mismatch")
	}
}

func TestScorerScore_SpecMismatch_differentFeatures(t *testing.T) {
	scorerSpec := testSpec()
	s := &Scorer{ens: new(mockModel), spec: scorerSpec}

	// Same version but different feature count.
	diffSpec := &domain.FeatureSpec{Version: "test-v1", Features: []domain.FeatureDef{{Name: "win_rate", Dtype: "float"}}}
	v := testVector(diffSpec, 1, []float64{0.5})
	_, err := s.Score(context.Background(), []*domain.FeatureVector{v})
	if err == nil {
		t.Fatal("expected error for spec mismatch with different features")
	}
}

func TestScorerScore_ModelError(t *testing.T) {
	mock := &mockModel{
		predictFn: func(_ []float64, _, _ int, _ []float64, _, _ int) error {
			return errors.New("predict failed")
		},
	}
	s := &Scorer{ens: mock, spec: testSpec()}
	v := testVector(testSpec(), 1, []float64{0.5, 0.3})
	_, err := s.Score(context.Background(), []*domain.FeatureVector{v})
	if err == nil {
		t.Fatal("expected error from model predict")
	}
}

func TestScorerScore_Success(t *testing.T) {
	mock := &mockModel{
		predictFn: func(_ []float64, nRows, nCols int, out []float64, startIter, numIter int) error {
			// Write one prediction per row: hero index as score
			for i := 0; i < nRows; i++ {
				out[i] = float64(i) + 1.0
			}
			return nil
		},
	}
	s := &Scorer{ens: mock, spec: testSpec()}
	spec := testSpec()

	vectors := []*domain.FeatureVector{
		testVector(spec, 10, []float64{0.5, 0.3}),
		testVector(spec, 20, []float64{0.7, 0.2}),
		testVector(spec, 30, []float64{0.9, 0.1}),
	}

	scores, err := s.Score(context.Background(), vectors)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(scores) != 3 {
		t.Fatalf("expected 3 scores, got %d", len(scores))
	}

	expected := []struct {
		hero  domain.HeroID
		value float64
	}{
		{10, 1.0},
		{20, 2.0},
		{30, 3.0},
	}
	for i, e := range expected {
		if scores[i].Hero != e.hero {
			t.Errorf("scores[%d].Hero = %d, want %d", i, scores[i].Hero, e.hero)
		}
		if scores[i].Value != e.value {
			t.Errorf("scores[%d].Value = %f, want %f", i, scores[i].Value, e.value)
		}
	}
}

// ──────────────────────────────────────────────
// ReloadableScorer
// ──────────────────────────────────────────────

func TestReloadableScorer_Nil(t *testing.T) {
	rs := NewReloadableScorer(nil)

	if rs.Load() != nil {
		t.Fatal("expected nil Load()")
	}
	if rs.Spec() != nil {
		t.Fatal("expected nil Spec()")
	}
	if rs.Version() != "" {
		t.Fatalf("expected empty Version, got %q", rs.Version())
	}
	if err := rs.Reload(); err != nil {
		t.Fatalf("Reload with nil underlying should not error: %v", err)
	}
	_, err := rs.Score(context.Background(), []*domain.FeatureVector{testVector(testSpec(), 1, []float64{0.5, 0.3})})
	if err == nil {
		t.Fatal("expected error when scoring with nil scorer")
	}
}

func TestReloadableScorer_WrapAndLoad(t *testing.T) {
	mock := &mockModel{
		predictFn: func(_ []float64, _, _ int, out []float64, _, _ int) error { return nil },
	}
	inner := &Scorer{ens: mock, spec: testSpec()}
	rs := NewReloadableScorer(inner)

	if rs.Load() != inner {
		t.Fatal("Load() should return the wrapped scorer")
	}
	if rs.Spec() != inner.spec {
		t.Fatal("Spec() should delegate")
	}
}

func TestReloadableScorer_Reload_noBin(t *testing.T) {
	// Reload fails gracefully when model.bin is missing.
	mock := &mockModel{
		predictFn: func(_ []float64, _, _ int, out []float64, _, _ int) error { return nil },
	}
	inner := &Scorer{ens: mock, spec: testSpec(), dir: t.TempDir()}

	// Write spec.json and meta.json but no model.bin.
	writeFile(t, filepath.Join(inner.dir, "spec.json"), `{"version":"v1","features":[]}`)
	writeFile(t, filepath.Join(inner.dir, "meta.json"), `{"version":"v1"}`)

	rs := NewReloadableScorer(inner)
	err := rs.Reload()
	if err == nil {
		t.Fatal("expected error when model.bin is missing")
	}

	// Original scorer should still be intact after failed reload.
	if rs.Load() != inner {
		t.Fatal("failed reload should not swap the pointer")
	}
}

// writeFile is a test helper.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// ──────────────────────────────────────────────
// LoadModel — file error paths
// ──────────────────────────────────────────────

func TestLoadModel_BadPath(t *testing.T) {
	_, err := LoadModel("/nonexistent/path")
	if err == nil {
		t.Fatal("expected error for bad path")
	}
}

func TestLoadModel_MissingSpec(t *testing.T) {
	dir := t.TempDir()
	// Write meta.json but no spec.json
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), []byte(`{"version":"v1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadModel(dir)
	if err == nil {
		t.Fatal("expected error for missing spec.json")
	}
}

func TestLoadModel_BadSpecJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "spec.json"), []byte(`not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), []byte(`{"version":"v1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadModel(dir)
	if err == nil {
		t.Fatal("expected error for bad spec.json")
	}
}

func TestLoadModel_BadMetaJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "spec.json"), []byte(`{"version":"v1","features":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), []byte(`not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadModel(dir)
	if err == nil {
		t.Fatal("expected error for bad meta.json")
	}
}

func TestLoadModel_MissingBin(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "spec.json"), []byte(`{"version":"v1","features":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), []byte(`{"version":"v1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// No model.bin — loadEnsemble will fail.
	_, err := LoadModel(dir)
	if err == nil {
		t.Fatal("expected error for missing model.bin")
	}
}

func TestLoadModel_LegacyTimestamp(t *testing.T) {
	// Legacy format: "20060102-150405" (Python trainer before fix).
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "spec.json"), []byte(`{"version":"v1","features":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "meta.json"),
		[]byte(`{"version":"v1","trained_at":"20260527-074946"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadModel(dir)
	if err == nil {
		t.Fatal("expected error (missing model.bin)")
	}
	// Error should be about model.bin, not timestamp parse failure.
	if !strings.Contains(err.Error(), "model.bin") {
		t.Fatalf("expected model.bin error, got: %v", err)
	}
}

func TestLoadModel_ISOTimestamp(t *testing.T) {
	// New ISO 8601 format: "2006-01-02T15:04:05Z" (Python trainer after fix).
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "spec.json"), []byte(`{"version":"v1","features":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "meta.json"),
		[]byte(`{"version":"v1","trained_at":"2026-05-27T07:49:46Z"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadModel(dir)
	if err == nil {
		t.Fatal("expected error (missing model.bin)")
	}
	if !strings.Contains(err.Error(), "model.bin") {
		t.Fatalf("expected model.bin error, got: %v", err)
	}
}

// ──────────────────────────────────────────────
// Explainer
// ──────────────────────────────────────────────

func specForTest(_ *testing.T) *domain.FeatureSpec {
	return &domain.FeatureSpec{
		Version: "test",
		Features: []domain.FeatureDef{
			{Name: "test_feat", Dtype: "f32"},
		},
	}
}

func TestExplainer_Explain(t *testing.T) {
	e := NewExplainer()
	vec := domain.NewFeatureVector(specForTest(t), domain.HeroID(10), []float64{1.0})
	reasons, risks, err := e.Explain(context.Background(), vec, 0.85)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reasons) != 1 {
		t.Fatalf("expected 1 reason, got %d", len(reasons))
	}
	if reasons[0].Factor != "model" {
		t.Errorf("expected factor 'model', got %q", reasons[0].Factor)
	}
	if reasons[0].Note != "score=0.8500" {
		t.Errorf("expected note 'score=0.8500', got %q", reasons[0].Note)
	}
	if risks != nil {
		t.Fatalf("expected nil risks, got %v", risks)
	}
}

func TestExplainer_ExplainZero(t *testing.T) {
	e := NewExplainer()
	vec := domain.NewFeatureVector(specForTest(t), domain.HeroID(5), []float64{0.0})
	reasons, risks, err := e.Explain(context.Background(), vec, 0.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reasons) != 1 {
		t.Fatalf("expected 1 reason, got %d", len(reasons))
	}
	if reasons[0].Note != "score=0.0000" {
		t.Errorf("expected note 'score=0.0000', got %q", reasons[0].Note)
	}
	if risks != nil {
		t.Fatalf("expected nil risks, got %v", risks)
	}
}
