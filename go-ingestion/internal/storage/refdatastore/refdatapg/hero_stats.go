package refdatapg

import (
	"context"
	"fmt"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/storage/refdatastore"
)

func (s *Store) UpsertHeroStats(ctx context.Context, hs []refdatastore.HeroStatRef) error {
	if len(hs) == 0 {
		return nil
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const q = `
		INSERT INTO hero_stats (id, pub_win_rate, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (id) DO UPDATE SET pub_win_rate = EXCLUDED.pub_win_rate, updated_at = NOW()`

	for _, stat := range hs {
		if _, err := tx.Exec(ctx, q, stat.HeroID, stat.Winrate); err != nil {
			return fmt.Errorf("upsert hero stat %d: %w", stat.HeroID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	s.log.Info("hero stats upserted", "count", len(hs))
	return nil
}