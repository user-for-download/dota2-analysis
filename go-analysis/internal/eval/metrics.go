package eval

import "math"

// PhaseMetrics holds evaluation metrics for a single draft phase.
type PhaseMetrics struct {
	Phase    string
	Total    int
	Correct  int
	Recall1  float64
	Recall3  float64
	Recall5  float64
	NDCG10   float64
}

// BacktestResult aggregates metrics across all phases.
type BacktestResult struct {
	PerPhase []PhaseMetrics
	Overall  PhaseMetrics
}

// computeRecall returns 1.0 if the actual hero rank is <= k, else 0.0.
func computeRecall(actualRank int, k int) float64 {
	if actualRank <= k {
		return 1.0
	}
	return 0.0
}

// computeNDCG10 computes NDCG@10 for a binary-relevance ranking.
// The actual picked hero has relevance 1, all others 0.
func computeNDCG10(actualRank int) float64 {
	if actualRank < 1 {
		return 0.0
	}
	// DCG: only the actual hero contributes (rel=1), at position actualRank.
	dcg := 1.0 / math.Log2(float64(actualRank+1))
	// IDCG: best case is the hero at rank 1.
	idcg := 1.0 / math.Log2(2.0) // = 1.0
	if actualRank > 10 {
		return 0.0
	}
	return dcg / idcg
}

// aggregateOverall computes the weighted average of per-phase metrics.
func aggregateOverall(perPhase []PhaseMetrics) PhaseMetrics {
	var total, correct int
	var sumR1, sumR3, sumR5, sumNDCG float64

	for _, pm := range perPhase {
		total += pm.Total
		correct += pm.Correct
		sumR1 += pm.Recall1 * float64(pm.Total)
		sumR3 += pm.Recall3 * float64(pm.Total)
		sumR5 += pm.Recall5 * float64(pm.Total)
		sumNDCG += pm.NDCG10 * float64(pm.Total)
	}

	if total == 0 {
		return PhaseMetrics{Phase: "overall"}
	}

	n := float64(total)
	return PhaseMetrics{
		Phase:   "overall",
		Total:   total,
		Correct: correct,
		Recall1: sumR1 / n,
		Recall3: sumR3 / n,
		Recall5: sumR5 / n,
		NDCG10:  sumNDCG / n,
	}
}
