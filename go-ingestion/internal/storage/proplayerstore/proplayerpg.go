package proplayerstore

import (
	"context"
	"fmt"
	"time"

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
INSERT INTO pro_players (account_id, steamid, personaname, name, country_code,
    fantasy_role, team_id, team_name, team_tag, is_pro, is_locked,
    avatar, last_match_time, last_login, full_history_time,
    cheese, fh_unavailable, loccountrycode, plus, updated_at)
SELECT *, now() FROM UNNEST(
    $1::bigint[], $2::text[], $3::text[], $4::text[], $5::text[],
    $6::int[], $7::bigint[], $8::text[], $9::text[], $10::bool[], $11::bool[],
    $12::text[], $13::timestamptz[], $14::timestamptz[], $15::timestamptz[],
    $16::int[], $17::bool[], $18::text[], $19::bool[]
)
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

	n := len(players)
	accountIDs := make([]int64, n)
	steamIDs := make([]*string, n)
	personanames := make([]*string, n)
	names := make([]*string, n)
	countryCodes := make([]*string, n)
	fantasyRoles := make([]*int, n)
	teamIDs := make([]*int64, n)
	teamNames := make([]*string, n)
	teamTags := make([]*string, n)
	isPros := make([]*bool, n)
	isLockeds := make([]*bool, n)
	avatars := make([]*string, n)
	lastMatchTimes := make([]*time.Time, n)
	lastLogins := make([]*time.Time, n)
	fullHistoryTimes := make([]*time.Time, n)
	cheeses := make([]*int, n)
	fhUnavailables := make([]*bool, n)
	locCountryCodes := make([]*string, n)
	pluses := make([]*bool, n)

	for i, p := range players {
		accountIDs[i] = p.AccountID
		steamIDs[i] = p.SteamID
		personanames[i] = p.Personaname
		names[i] = p.Name
		countryCodes[i] = p.CountryCode
		fantasyRoles[i] = p.FantasyRole
		teamIDs[i] = p.TeamID
		teamNames[i] = p.TeamName
		teamTags[i] = p.TeamTag
		isPros[i] = p.IsPro
		isLockeds[i] = p.IsLocked
		avatars[i] = p.Avatar
		lastMatchTimes[i] = p.LastMatchTime
		lastLogins[i] = p.LastLogin
		fullHistoryTimes[i] = p.FullHistoryTime
		cheeses[i] = p.Cheese
		fhUnavailables[i] = p.FhUnavailable
		locCountryCodes[i] = p.LocCountryCode
		pluses[i] = p.Plus
	}

	_, err := r.pool.Exec(ctx, upsertBulkSQL,
		accountIDs, steamIDs, personanames, names, countryCodes,
		fantasyRoles, teamIDs, teamNames, teamTags, isPros, isLockeds,
		avatars, lastMatchTimes, lastLogins, fullHistoryTimes,
		cheeses, fhUnavailables, locCountryCodes, pluses,
	)
	if err != nil {
		return 0, fmt.Errorf("bulk upsert pro players: %w", err)
	}
	return n, nil
}
