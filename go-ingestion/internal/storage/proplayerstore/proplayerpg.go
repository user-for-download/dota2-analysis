package proplayerstore

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
INSERT INTO pro_players (account_id, steamid, personaname, name, country_code,
    fantasy_role, team_id, team_name, team_tag, is_pro, is_locked,
    avatar, last_match_time, last_login, full_history_time,
    cheese, fh_unavailable, loccountrycode, plus, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, now())
ON CONFLICT (account_id) DO UPDATE SET
    steamid = EXCLUDED.steamid,
    personaname = EXCLUDED.personaname,
    name = EXCLUDED.name,
    country_code = EXCLUDED.country_code,
    fantasy_role = EXCLUDED.fantasy_role,
    team_id = EXCLUDED.team_id,
    team_name = EXCLUDED.team_name,
    team_tag = EXCLUDED.team_tag,
    is_pro = EXCLUDED.is_pro,
    is_locked = EXCLUDED.is_locked,
    avatar = EXCLUDED.avatar,
    last_match_time = EXCLUDED.last_match_time,
    last_login = EXCLUDED.last_login,
    full_history_time = EXCLUDED.full_history_time,
    cheese = EXCLUDED.cheese,
    fh_unavailable = EXCLUDED.fh_unavailable,
    loccountrycode = EXCLUDED.loccountrycode,
    plus = EXCLUDED.plus,
    updated_at = now()
`

func (r *PG) Upsert(ctx context.Context, players []ProPlayer) (int, error) {
	if len(players) == 0 {
		return 0, nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var n int
	for _, p := range players {
		if _, err := tx.Exec(ctx, upsertSQL,
			p.AccountID, p.SteamID, p.Personaname, p.Name, p.CountryCode,
			p.FantasyRole, p.TeamID, p.TeamName, p.TeamTag, p.IsPro, p.IsLocked,
			p.Avatar, p.LastMatchTime, p.LastLogin, p.FullHistoryTime,
			p.Cheese, p.FhUnavailable, p.LocCountryCode, p.Plus,
		); err != nil {
			return n, fmt.Errorf("upsert pro player %d: %w", p.AccountID, err)
		}
		n++
	}
	if err := tx.Commit(ctx); err != nil {
		return n, fmt.Errorf("commit: %w", err)
	}
	return n, nil
}