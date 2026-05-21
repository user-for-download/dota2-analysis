package refdatapg

import (
	"context"
	"fmt"

	"github.com/user-for-download/go-dota2/internal/storage/refdatastore"
)

func (s *Store) UpsertNotablePlayers(ctx context.Context, np []refdatastore.NotablePlayerRef) error {
	if len(np) == 0 {
		return nil
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const q = `
		INSERT INTO notable_players (account_id, name, team_id, country_code, is_pro, updated_at)
		VALUES ($1, $2, NULLIF($3, 0), $4, FALSE, NOW())
		ON CONFLICT (account_id) DO UPDATE SET
			name = EXCLUDED.name, team_id = EXCLUDED.team_id, country_code = EXCLUDED.country_code, updated_at = NOW()`

	for _, p := range np {
		if _, err := tx.Exec(ctx, q, p.AccountID, p.Name, p.TeamID, p.Country); err != nil {
			return fmt.Errorf("upsert notable player %d: %w", p.AccountID, err)
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) UpsertProPlayers(ctx context.Context, pp []refdatastore.ProPlayerRef) error {
	if len(pp) == 0 {
		return nil
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const q = `
		INSERT INTO notable_players (account_id, name, team_id, country_code, is_pro, updated_at)
		VALUES ($1, $2, NULLIF($3, 0), $4, TRUE, NOW())
		ON CONFLICT (account_id) DO UPDATE SET
			name = EXCLUDED.name, team_id = EXCLUDED.team_id, country_code = EXCLUDED.country_code, is_pro = TRUE, updated_at = NOW()`

	for _, p := range pp {
		if _, err := tx.Exec(ctx, q, p.AccountID, p.Name, p.TeamID, p.Country); err != nil {
			return fmt.Errorf("upsert pro player %d: %w", p.AccountID, err)
		}
	}
	return tx.Commit(ctx)
}