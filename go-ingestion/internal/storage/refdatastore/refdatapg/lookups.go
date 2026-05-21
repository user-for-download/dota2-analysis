package refdatapg

import (
	"context"
	"fmt"

	"github.com/user-for-download/go-dota2/internal/storage/refdatastore"
)

func (s *Store) UpsertGameModes(ctx context.Context, gm []refdatastore.GameModeRef) error {
	if len(gm) == 0 {
		return nil
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const q = `
		INSERT INTO game_modes (id, name, balanced, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, balanced = EXCLUDED.balanced, updated_at = NOW()`

	for _, g := range gm {
		if g.ID < 0 || g.ID > 32767 || g.Name == "" {
			continue
		}
		if _, err := tx.Exec(ctx, q, int16(g.ID), g.Name, g.Balanced); err != nil {
			return fmt.Errorf("upsert game_mode %d: %w", g.ID, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	s.log.Info("game_modes upserted", "count", len(gm))
	return nil
}

func (s *Store) UpsertLobbyTypes(ctx context.Context, lt []refdatastore.LobbyTypeRef) error {
	if len(lt) == 0 {
		return nil
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const q = `
		INSERT INTO lobby_types (id, name, balanced, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, balanced = EXCLUDED.balanced, updated_at = NOW()`

	for _, l := range lt {
		if l.ID < 0 || l.ID > 32767 || l.Name == "" {
			continue
		}
		if _, err := tx.Exec(ctx, q, int16(l.ID), l.Name, l.Balanced); err != nil {
			return fmt.Errorf("upsert lobby_type %d: %w", l.ID, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	s.log.Info("lobby_types upserted", "count", len(lt))
	return nil
}

func (s *Store) UpsertRegions(ctx context.Context, reg []refdatastore.RegionRef) error {
	if len(reg) == 0 {
		return nil
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const q = `
		INSERT INTO regions (id, name, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, updated_at = NOW()`

	for _, re := range reg {
		if re.ID < 0 || re.ID > 32767 || re.Name == "" {
			continue
		}
		if _, err := tx.Exec(ctx, q, int16(re.ID), re.Name); err != nil {
			return fmt.Errorf("upsert region %d: %w", re.ID, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	s.log.Info("regions upserted", "count", len(reg))
	return nil
}