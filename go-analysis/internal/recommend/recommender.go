package recommend

import (
	"context"
	"log/slog"
	"sort"

	"github.com/user-for-download/go-dota2-analysis/internal/domain"
	"github.com/user-for-download/go-dota2-analysis/internal/features"
	"github.com/user-for-download/go-dota2-analysis/internal/scoring"
)

// Recommender generates ranked hero recommendations for a draft state.
type Recommender interface {
	Recommend(ctx context.Context, st *domain.DraftState, k int) (*domain.Result, error)
}

// Service implements Recommender.
type Service struct {
	builder     features.Builder
	scorer      scoring.Scorer
	valueScorer scoring.Scorer // optional, nil if not configured
	explainer   scoring.Explainer
	catalog     domain.HeroCatalog
	ensemble    *EnsembleScore
	log         *slog.Logger
}

// NewService creates a recommendation service.
func NewService(builder features.Builder, scorer scoring.Scorer, explainer scoring.Explainer, catalog domain.HeroCatalog) *Service {
	return &Service{
		builder:   builder,
		scorer:    scorer,
		explainer: explainer,
		catalog:   catalog,
		ensemble:  DefaultEnsemble(),
		log:       slog.Default(),
	}
}

// WithLog returns a new Service with the given logger attached.
func (s *Service) WithLog(log *slog.Logger) *Service {
	cp := *s
	cp.log = log
	return &cp
}

// WithValueScorer returns a new Service with a value model scorer and balanced ensemble.
func (s *Service) WithValueScorer(valueScorer scoring.Scorer) *Service {
	return &Service{
		builder:     s.builder,
		scorer:      s.scorer,
		valueScorer: valueScorer,
		explainer:   s.explainer,
		catalog:     s.catalog,
		ensemble:    BalancedEnsemble(),
		log:         s.log,
	}
}

// Recommend returns the top-K hero recommendations for the current draft state.
func (s *Service) Recommend(ctx context.Context, st *domain.DraftState, k int) (*domain.Result, error) {
	// 1. Generate candidates (legal heroes not yet drafted)
	candidates := st.LegalHeroes(s.catalog)
	if len(candidates) == 0 {
		return &domain.Result{}, nil
	}

	// 2. Build feature vectors
	vectors, err := s.builder.Build(ctx, st, candidates)
	if err != nil {
		return nil, err
	}

	// 3. Score with imitation model
	imitationScores, err := s.scorer.Score(ctx, vectors)
	if err != nil {
		return nil, err
	}

	// 4. Score with value model if available (graceful fallback)
	var valueScores []domain.Score
	var warnings []string
	usedValueModel := s.valueScorer != nil
	if usedValueModel {
		valueScores, err = s.valueScorer.Score(ctx, vectors)
		if err != nil {
			s.log.Warn("value scorer failed, using imitation only", "err", err)
			warnings = append(warnings, "value model unavailable: "+err.Error())
			valueScores = nil
		}
	}

	// 5. Combine scores via ensemble
	finalScores := s.ensemble.Combine(imitationScores, valueScores)

	// 6. Sort by score descending
	sort.Slice(finalScores, func(i, j int) bool { return finalScores[i].Value > finalScores[j].Value })

	// 7. Take top-K
	if k <= 0 || k > len(finalScores) {
		k = len(finalScores)
	}
	topK := finalScores[:k]

	// 8. Build recommendations with explanations
	recs := make([]domain.Recommendation, k)
	for i, sc := range topK {
		reasons, riskReasons, _ := s.explainer.Explain(ctx, sc.Hero, sc.Value)
		recs[i] = domain.Recommendation{
			Hero:    sc.Hero,
			Name:    s.catalog.Name(sc.Hero),
			Score:   sc.Value,
			Rank:    i + 1,
			Reasons: reasons,
			Risks:   reasonsFromRiskReasons(riskReasons),
		}
	}

	return &domain.Result{
		Recommendations: recs,
		UsedValueModel:  usedValueModel,
		Warnings:        warnings,
	}, nil
}

// reasonsFromRiskReasons converts []domain.Reason risk reasons to []string.
func reasonsFromRiskReasons(rs []domain.Reason) []string {
	if len(rs) == 0 {
		return nil
	}
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Factor + ": " + r.Note
	}
	return out
}
