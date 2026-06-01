package matchpg

import (
	"context"
	"sort"

	"github.com/jackc/pgx/v5"

	"github.com/user-for-download/dota2-analysis/go-core/domain"
)

func collectHeroIDs(m domain.Match) []int16 {
	seen := make(map[int16]struct{})
	var ids []int16

	for _, p := range m.Players {
		h := int16(p.HeroID)
		if h != 0 {
			if _, ok := seen[h]; !ok {
				seen[h] = struct{}{}
				ids = append(ids, h)
			}
		}
	}
	for _, pb := range m.PicksBans {
		h := int16(pb.HeroID)
		if h != 0 {
			if _, ok := seen[h]; !ok {
				seen[h] = struct{}{}
				ids = append(ids, h)
			}
		}
	}
	for _, dt := range m.DraftTimings {
		h := int16(dt.HeroID)
		if h != 0 {
			if _, ok := seen[h]; !ok {
				seen[h] = struct{}{}
				ids = append(ids, h)
			}
		}
	}
	return ids
}

func (s *Store) ensureHeroStubs(ctx context.Context, tx pgx.Tx, heroIDs []int16) error {
	if len(heroIDs) == 0 {
		return nil
	}
	// Sort by ID to guarantee consistent lock acquisition order across all
	// concurrent transactors (enricher, other parser workers).  Without this,
	// player_matches FK checks and ensure_hero_stubs each lock hero rows in
	// the order they appear — different orders cause deadlock (SQLSTATE 40P01).
	sort.Slice(heroIDs, func(i, j int) bool { return heroIDs[i] < heroIDs[j] })
	_, err := tx.Exec(ctx, "SELECT ensure_hero_stubs($1)", heroIDs)
	return err
}