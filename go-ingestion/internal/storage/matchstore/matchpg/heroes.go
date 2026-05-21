package matchpg

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/user-for-download/go-dota2/internal/storage/matchstore"
)

func collectHeroIDs(m matchstore.Match) []int16 {
	seen := make(map[int16]struct{})
	var ids []int16

	for _, p := range m.Players {
		if p.HeroID != 0 {
			if _, ok := seen[p.HeroID]; !ok {
				seen[p.HeroID] = struct{}{}
				ids = append(ids, p.HeroID)
			}
		}
	}
	for _, pb := range m.PicksBans {
		if pb.HeroID != 0 {
			if _, ok := seen[pb.HeroID]; !ok {
				seen[pb.HeroID] = struct{}{}
				ids = append(ids, pb.HeroID)
			}
		}
	}
	for _, dt := range m.DraftTimings {
		if dt.HeroID != 0 {
			if _, ok := seen[dt.HeroID]; !ok {
				seen[dt.HeroID] = struct{}{}
				ids = append(ids, dt.HeroID)
			}
		}
	}
	return ids
}

func (s *Store) ensureHeroStubs(ctx context.Context, tx pgx.Tx, heroIDs []int16) error {
	if len(heroIDs) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, "SELECT ensure_hero_stubs($1)", heroIDs)
	return err
}