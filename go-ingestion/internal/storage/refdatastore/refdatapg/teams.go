package refdatapg

import (
	"context"
	"fmt"

	"github.com/user-for-download/go-dota2/internal/storage/refdatastore"
)

func (s *Store) UpsertTeams(ctx context.Context, teams []refdatastore.TeamRef) error {
	if len(teams) == 0 {
		return nil
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const q = `
		INSERT INTO teams (team_id, name, tag, logo_url, updated_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (team_id) DO UPDATE SET name = EXCLUDED.name, tag = EXCLUDED.tag, logo_url = EXCLUDED.logo_url, updated_at = NOW()`

	for _, t := range teams {
		if _, err := tx.Exec(ctx, q, t.ID, t.Name, t.Tag, t.LogoURL); err != nil {
			return fmt.Errorf("upsert team %d: %w", t.ID, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	s.log.Info("teams upserted", "count", len(teams))
	return nil
}

func (s *Store) UpsertLeagues(ctx context.Context, leagues []refdatastore.LeagueRef) error {
	if len(leagues) == 0 {
		return nil
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const q = `
		INSERT INTO leagues (leagueid, name, tier, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (leagueid) DO UPDATE SET name = EXCLUDED.name, tier = EXCLUDED.tier, updated_at = NOW()`

	for _, l := range leagues {
		if _, err := tx.Exec(ctx, q, l.ID, l.Name, l.Tier); err != nil {
			return fmt.Errorf("upsert league %d: %w", l.ID, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	s.log.Info("leagues upserted", "count", len(leagues))
	return nil
}