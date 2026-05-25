package recommend

import (
	"github.com/user-for-download/dota2-analysis/go-analysis/internal/domain"
)

// CandidateGen generates candidate heroes for recommendation.
type CandidateGen struct {
	Catalog domain.HeroCatalog
}

// Generate returns all legal (not drafted/banned) heroes.
// This must match the Python trainer's candidate generation.
func (g *CandidateGen) Generate(st *domain.DraftState) []domain.HeroID {
	return st.LegalHeroes(g.Catalog)
}
