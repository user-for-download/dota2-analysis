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

const upsertBulkSQL = `
INSERT INTO teams (team_id, name, tag, logo_url, rating, wins, losses, last_match_time, delta, match_id)
SELECT * FROM UNNEST($1::bigint[], $2::text[], $3::text[], $4::text[],
	$5::float8[], $6::int[], $7::int[], $8::bigint[], $9::float8[], $10::bigint[])
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

	n := len(teams)
	teamIDs := make([]int64, n)
	names := make([]string, n)
	tags := make([]string, n)
	logoURLs := make([]string, n)
	ratings := make([]*float64, n)
	wins := make([]*int, n)
	losses := make([]*int, n)
	lastMatchTimes := make([]*int64, n)
	deltas := make([]*float64, n)
	matchIDs := make([]*int64, n)

	for i, t := range teams {
		teamIDs[i] = t.TeamID
		names[i] = t.Name
		tags[i] = t.Tag
		logoURLs[i] = t.LogoURL
		ratings[i] = t.Rating
		wins[i] = t.Wins
		losses[i] = t.Losses
		lastMatchTimes[i] = t.LastMatchTime
		deltas[i] = t.Delta
		matchIDs[i] = t.MatchID
	}

	_, err := r.pool.Exec(ctx, upsertBulkSQL,
		teamIDs, names, tags, logoURLs,
		ratings, wins, losses, lastMatchTimes, deltas, matchIDs,
	)
	if err != nil {
		return 0, fmt.Errorf("bulk upsert teams: %w", err)
	}
	return n, nil
}
