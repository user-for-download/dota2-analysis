package refdatapg

import (
	"context"
	"fmt"

	"github.com/user-for-download/go-dota2/internal/storage/refdatastore"
)

func (s *Store) UpsertPatches(ctx context.Context, p []refdatastore.PatchRef) error {
	if len(p) == 0 {
		return nil
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const q = `
		INSERT INTO patches (id, name, release_at, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, release_at = EXCLUDED.release_at, updated_at = NOW()`

	for _, pat := range p {
		if pat.ID <= 0 || pat.Name == "" {
			continue
		}
		if _, err := tx.Exec(ctx, q, pat.ID, pat.Name, pat.ReleaseAt); err != nil {
			return fmt.Errorf("upsert patch %d: %w", pat.ID, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	s.log.Info("patches upserted", "count", len(p))
	return nil
}