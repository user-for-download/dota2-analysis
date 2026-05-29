package refdatapg

import (
	"context"
	"fmt"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/storage/refdatastore"
)

func (s *Store) UpsertItems(ctx context.Context, items []refdatastore.ItemRef) error {
	if len(items) == 0 {
		return nil
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const q = `
		INSERT INTO items (id, key, dname, cost, qual, behavior, lore, img, created, cooldown, mana_cost, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW())
		ON CONFLICT (id) DO UPDATE SET
			key = EXCLUDED.key, dname = EXCLUDED.dname, cost = EXCLUDED.cost, qual = EXCLUDED.qual,
			behavior = EXCLUDED.behavior, lore = EXCLUDED.lore, img = EXCLUDED.img, created = EXCLUDED.created,
			cooldown = EXCLUDED.cooldown, mana_cost = EXCLUDED.mana_cost, updated_at = NOW()`

	for _, it := range items {
		if it.ID <= 0 || it.Key == "" {
			continue
		}
		if _, err := tx.Exec(ctx, q, it.ID, it.Key, it.DName, it.Cost, it.Qual,
			it.Behavior, it.Lore, it.Img, it.Created, it.Cooldown, it.ManaCost); err != nil {
			return fmt.Errorf("upsert item %d: %w", it.ID, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	s.log.Info("items upserted", "count", len(items))
	return nil
}

func (s *Store) UpsertItemIDs(ctx context.Context, ii []refdatastore.ItemIDRef) error {
	if len(ii) == 0 {
		return nil
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const q = `
		INSERT INTO item_ids (id, key, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (id) DO UPDATE SET key = EXCLUDED.key, updated_at = NOW()`

	for _, i := range ii {
		if i.ID < 0 || i.Key == "" {
			continue
		}
		if _, err := tx.Exec(ctx, q, i.ID, i.Key); err != nil {
			return fmt.Errorf("upsert item_id %d: %w", i.ID, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	s.log.Info("item_ids upserted", "count", len(ii))
	return nil
}