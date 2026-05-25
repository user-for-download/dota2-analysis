package eval

import (
	"math"
	"testing"

	"github.com/user-for-download/go-dota2-analysis/internal/domain"
)

// ──────────────────────────────────────────────
// computeRecall
// ──────────────────────────────────────────────

func TestComputeRecall(t *testing.T) {
	tests := []struct {
		rank int
		k    int
		want float64
	}{
		{1, 1, 1.0},  // top-1 at k=1 → hit
		{1, 3, 1.0},  // top-1 at k=3 → hit
		{2, 1, 0.0},  // rank 2 at k=1 → miss
		{3, 3, 1.0},  // rank 3 at k=3 → hit
		{4, 3, 0.0},  // rank 4 at k=3 → miss
		{1, 5, 1.0},  // top-1 always hit
		{5, 5, 1.0},  // rank 5 at k=5 → hit
		{10, 5, 0.0}, // rank 10 at k=5 → miss
	}
	for _, tc := range tests {
		got := computeRecall(tc.rank, tc.k)
		if got != tc.want {
			t.Errorf("computeRecall(%d, %d) = %f, want %f", tc.rank, tc.k, got, tc.want)
		}
	}
}

// ──────────────────────────────────────────────
// computeNDCG10
// ──────────────────────────────────────────────

func TestComputeNDCG10(t *testing.T) {
	tests := []struct {
		rank int
		want float64
	}{
		{-1, 0.0}, // invalid rank
		{0, 0.0},  // invalid rank
		{1, 1.0},  // DCG@1 / IDCG@1 = (1/log2(2)) / (1/log2(2)) = 1
		{2, 1.0 / math.Log2(3)}, // DCG@2 = 1/log2(3), IDCG = 1, so NDCG = 1/log2(3)
		{3, 1.0 / math.Log2(4)}, // 1/log2(4)
		{5, 1.0 / math.Log2(6)}, // 1/log2(6)
		{10, 1.0 / math.Log2(11)}, // 1/log2(11)
		{11, 0.0}, // beyond rank 10 → 0
		{20, 0.0}, // far beyond → 0
	}
	for _, tc := range tests {
		got := computeNDCG10(tc.rank)
		if math.Abs(got-tc.want) > 1e-12 {
			t.Errorf("computeNDCG10(%d) = %.15f, want %.15f", tc.rank, got, tc.want)
		}
	}
}

// ──────────────────────────────────────────────
// aggregateOverall
// ──────────────────────────────────────────────

func TestAggregateOverall_singlePhase(t *testing.T) {
	pm := []PhaseMetrics{
		{Phase: "phase1", Total: 10, Correct: 5, Recall1: 0.5, Recall3: 0.7, Recall5: 0.9, NDCG10: 0.6},
	}
	overall := aggregateOverall(pm)

	if overall.Phase != "overall" {
		t.Errorf("Phase = %q, want overall", overall.Phase)
	}
	if overall.Total != 10 {
		t.Errorf("Total = %d, want 10", overall.Total)
	}
	if overall.Correct != 5 {
		t.Errorf("Correct = %d, want 5", overall.Correct)
	}
	if math.Abs(overall.Recall1-0.5) > 1e-12 {
		t.Errorf("Recall1 = %f, want 0.5", overall.Recall1)
	}
}

func TestAggregateOverall_multiPhase(t *testing.T) {
	pm := []PhaseMetrics{
		{Phase: "p1", Total: 20, Correct: 10, Recall1: 0.5, Recall3: 0.6, Recall5: 0.7, NDCG10: 0.55},
		{Phase: "p2", Total: 30, Correct: 15, Recall1: 0.3, Recall3: 0.5, Recall5: 0.8, NDCG10: 0.45},
	}
	overall := aggregateOverall(pm)

	wantTotal := 50
	if overall.Total != wantTotal {
		t.Errorf("Total = %d, want %d", overall.Total, wantTotal)
	}
	wantCorrect := 25
	if overall.Correct != wantCorrect {
		t.Errorf("Correct = %d, want %d", overall.Correct, wantCorrect)
	}
	// Weighted averages
	wantR1 := (0.5*20 + 0.3*30) / 50.0
	wantR3 := (0.6*20 + 0.5*30) / 50.0
	wantR5 := (0.7*20 + 0.8*30) / 50.0
	wantNDCG := (0.55*20 + 0.45*30) / 50.0

	check := func(name string, got, want float64) {
		if math.Abs(got-want) > 1e-12 {
			t.Errorf("%s = %.10f, want %.10f", name, got, want)
		}
	}
	check("Recall1", overall.Recall1, wantR1)
	check("Recall3", overall.Recall3, wantR3)
	check("Recall5", overall.Recall5, wantR5)
	check("NDCG10", overall.NDCG10, wantNDCG)
}

func TestAggregateOverall_empty(t *testing.T) {
	overall := aggregateOverall(nil)

	if overall.Total != 0 {
		t.Errorf("Total = %d, want 0", overall.Total)
	}
	if overall.Correct != 0 {
		t.Errorf("Correct = %d, want 0", overall.Correct)
	}
}

func TestAggregateOverall_noData(t *testing.T) {
	overall := aggregateOverall([]PhaseMetrics{
		{Phase: "empty", Total: 0, Correct: 0},
	})

	if overall.Total != 0 {
		t.Errorf("Total = %d, want 0", overall.Total)
	}
}

// ──────────────────────────────────────────────
// uniformScores
// ──────────────────────────────────────────────

func TestUniformScores_basic(t *testing.T) {
	candidates := []domain.HeroID{1, 5, 10}
	scores := uniformScores(candidates)

	if len(scores) != 3 {
		t.Fatalf("got %d scores, want 3", len(scores))
	}
	for _, s := range scores {
		if s.Value != 0.5 {
			t.Errorf("hero %d has score %f, want 0.5", s.Hero, s.Value)
		}
	}
	// Order preserved
	if scores[0].Hero != 1 || scores[1].Hero != 5 || scores[2].Hero != 10 {
		t.Errorf("order not preserved: got %v, want [1 5 10]", scores)
	}
}

func TestUniformScores_empty(t *testing.T) {
	// nil input → make([]T, 0) returns empty slice, not nil.
	scores := uniformScores(nil)
	if scores == nil {
		t.Fatal("uniformScores(nil) should return non-nil empty slice")
	}
	if len(scores) != 0 {
		t.Fatalf("expected 0 scores, got %d", len(scores))
	}

	scores = uniformScores([]domain.HeroID{})
	if scores == nil || len(scores) != 0 {
		t.Fatalf("expected empty slice for empty input, got %v", scores)
	}
}

// ──────────────────────────────────────────────
// UniformRandomBaseline
// ──────────────────────────────────────────────

func TestUniformRandomBaseline(t *testing.T) {
	var b UniformRandomBaseline
	candidates := []domain.HeroID{1, 2, 3}
	scores := b.Score(candidates)

	if len(scores) != 3 {
		t.Fatalf("got %d scores, want 3", len(scores))
	}
	// Values should be in [0, 1) — can't assert exact values since random.
	for i, s := range scores {
		if s.Hero != candidates[i] {
			t.Errorf("scores[%d].Hero = %d, want %d", i, s.Hero, candidates[i])
		}
		if s.Value < 0 || s.Value >= 1.0 {
			t.Errorf("scores[%d].Value = %f, out of [0,1)", i, s.Value)
		}
	}
}

// ──────────────────────────────────────────────
// PickFrequencyBaseline.Score
// ──────────────────────────────────────────────

func TestPickFrequencyBaseline_Score(t *testing.T) {
	b := &PickFrequencyBaseline{
		Freqs: map[domain.HeroID]float64{
			1: 0.3,
			2: 0.5,
			3: 0.2,
		},
	}

	// All candidates have known frequencies.
	scores := b.Score([]domain.HeroID{1, 2, 3})
	if len(scores) != 3 {
		t.Fatalf("got %d scores, want 3", len(scores))
	}
	expect := map[domain.HeroID]float64{1: 0.3, 2: 0.5, 3: 0.2}
	for _, s := range scores {
		if s.Value != expect[s.Hero] {
			t.Errorf("hero %d: got %f, want %f", s.Hero, s.Value, expect[s.Hero])
		}
	}
}

func TestPickFrequencyBaseline_Score_missingHeroes(t *testing.T) {
	b := &PickFrequencyBaseline{
		Freqs: map[domain.HeroID]float64{1: 0.8},
	}

	// Hero 5 has no frequency → defaults to 0.
	scores := b.Score([]domain.HeroID{1, 5})
	if len(scores) != 2 {
		t.Fatalf("got %d scores, want 2", len(scores))
	}
	if scores[0].Value != 0.8 {
		t.Errorf("hero 1: got %f, want 0.8", scores[0].Value)
	}
	if scores[1].Value != 0.0 {
		t.Errorf("hero 5: got %f, want 0.0", scores[1].Value)
	}
}

func TestPickFrequencyBaseline_Score_empty(t *testing.T) {
	b := &PickFrequencyBaseline{}
	scores := b.Score(nil)
	if scores == nil {
		t.Fatal("expected non-nil empty slice for nil input")
	}
	if len(scores) != 0 {
		t.Fatalf("expected 0 scores, got %v", scores)
	}

	scores = b.Score([]domain.HeroID{})
	if scores == nil || len(scores) != 0 {
		t.Fatalf("expected empty slice for empty input, got %v", scores)
	}
}

// ──────────────────────────────────────────────
// PlayerComfortBaseline.Score (no DB needed)
// ──────────────────────────────────────────────

func TestPlayerComfortBaseline_Score_fallbackToUniform(t *testing.T) {
	// Without a DB, Score falls back to uniform via uniformScores.
	b := &PlayerComfortBaseline{db: nil}
	candidates := []domain.HeroID{1, 2}
	scores := b.Score(candidates)

	if len(scores) != 2 {
		t.Fatalf("got %d scores, want 2", len(scores))
	}
	for _, s := range scores {
		if s.Value != 0.5 {
			t.Errorf("hero %d: got %f, want 0.5", s.Hero, s.Value)
		}
	}
}
