package teamstore

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PG struct {
	pool *pgxpool.Pool
}

func NewPG(p *pgxpool.Pool) *PG {
	return &PG{pool: p}
}

var _ Writer = (*PG)(nil)

const upsertSQL = `
INSERT INTO teams (team_id, name, tag, logo_url, rating, wins, losses, last_match_time, delta, match_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (team_id) DO UPDATE
SET name            = EXCLUDED.name,
    tag             = EXCLUDED.tag,
    logo_url        = EXCLUDED.logo_url,
    rating          = EXCLUDED.rating,
    wins            = EXCLUDED.wins,
    losses          = EXCLUDED.losses,
    last_match_time = EXCLUDED.last_match_time,
    delta           = EXCLUDED.delta,
    match_id        = EXCLUDED.match_id
`

func (r *PG) Upsert(ctx context.Context, teams []Team) (int, error) {
	if len(teams) == 0 {
		return 0, nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var n int
	for _, t := range teams {
		if _, err := tx.Exec(ctx, upsertSQL,
			t.TeamID, t.Name, t.Tag, t.LogoURL,
			t.Rating, t.Wins, t.Losses, t.LastMatchTime, t.Delta, t.MatchID,
		); err != nil {
			return n, fmt.Errorf("upsert team %d: %w", t.TeamID, err)
		}
		n++
	}
	if err := tx.Commit(ctx); err != nil {
		return n, fmt.Errorf("commit: %w", err)
	}
	return n, nil
}