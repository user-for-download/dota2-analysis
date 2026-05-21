package refdatapg

import (
	"context"
	"fmt"

	"github.com/user-for-download/go-dota2/internal/storage/refdatastore"
)

func (s *Store) UpsertHeroes(ctx context.Context, heroes []refdatastore.HeroRef) error {
	if len(heroes) == 0 {
		return nil
	}

	seenName := make(map[string]int, len(heroes))
	for _, h := range heroes {
		if h.Name == "" {
			continue
		}
		if prev, dup := seenName[h.Name]; dup {
			return fmt.Errorf("duplicate hero name %q for ids %d and %d", h.Name, prev, h.ID)
		}
		seenName[h.Name] = h.ID
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const q = `
		INSERT INTO heroes (id, name, localized_name, primary_attr, attack_type, roles, legs, updated_at)
		VALUES ($1, $2, $3, NULLIF($4,''), NULLIF($5,''), $6, $7::SMALLINT, NOW())
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name, localized_name = EXCLUDED.localized_name, primary_attr = EXCLUDED.primary_attr,
			attack_type = EXCLUDED.attack_type, roles = EXCLUDED.roles, legs = EXCLUDED.legs, updated_at = NOW()`

	var applied, skipped int
	for _, h := range heroes {
		if h.ID <= 0 || h.ID > 32767 {
			skipped++
			continue
		}
		if _, err := tx.Exec(ctx, q,
			int16(h.ID), h.Name, h.LocalizedName, h.PrimaryAttr, h.AttackType, h.Roles, h.Legs,
		); err != nil {
			return fmt.Errorf("upsert hero id=%d: %w", h.ID, err)
		}
		applied++
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	s.log.Info("heroes upserted", "applied", applied, "skipped", skipped, "input", len(heroes))
	return nil
}