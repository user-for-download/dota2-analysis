package leaguestore

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
INSERT INTO leagues (leagueid, name, tier, ticket, banner, updated_at)
VALUES ($1, $2, $3, $4, $5, now())
ON CONFLICT (leagueid) DO UPDATE
SET name       = EXCLUDED.name,
    tier       = EXCLUDED.tier,
    ticket     = EXCLUDED.ticket,
    banner     = EXCLUDED.banner,
    updated_at = now()
`

func (r *PG) Upsert(ctx context.Context, leagues []League) (int, error) {
	if len(leagues) == 0 {
		return 0, nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var n int
	for _, l := range leagues {
		if _, err := tx.Exec(ctx, upsertSQL,
			l.LeagueID, l.Name, l.Tier, l.Ticket, l.Banner,
		); err != nil {
			return n, fmt.Errorf("upsert league %d: %w", l.LeagueID, err)
		}
		n++
	}
	if err := tx.Commit(ctx); err != nil {
		return n, fmt.Errorf("commit: %w", err)
	}
	return n, nil
}