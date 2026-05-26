package postgres

import (
	"context"
	"fmt"

	"github.com/user-for-download/dota2-analysis/go-analysis/internal/domain"
	"github.com/user-for-download/dota2-analysis/go-analysis/internal/profiles"
)

// PlayerHeroes returns a player's historical performance with each hero.
func (r *PGRepository) PlayerHeroes(ctx context.Context, accountID domain.AccountID, minGames, limit int) ([]profiles.PlayerHero, error) {
	rows, err := r.db.Query(ctx, `
		SELECT hero_id, hero_name, games, wins, shrunk_wr, refreshed_at
		FROM analytics.mv_player_hero_profile
		WHERE account_id = $1 AND games >= $2
		ORDER BY games DESC
		LIMIT $3
	`, accountID, minGames, limit)
	if err != nil {
		return nil, fmt.Errorf("player heroes: %w", err)
	}
	defer rows.Close()

	var out []profiles.PlayerHero
	for rows.Next() {
		var p profiles.PlayerHero
		if err := rows.Scan(&p.HeroID, &p.HeroName, &p.Games, &p.Wins, &p.WRShrunk, &p.LastPlayed); err != nil {
			return nil, fmt.Errorf("scan player heroes: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// PlayerTeams returns a player's recent team affiliations.
func (r *PGRepository) PlayerTeams(ctx context.Context, accountID domain.AccountID, limit int) ([]profiles.PlayerTeam, error) {
	rows, err := r.db.Query(ctx, `
		SELECT team_id, games, wins, last_match_time, last_patch_id
		FROM analytics.mv_player_team_history
		WHERE account_id = $1
		ORDER BY games DESC
		LIMIT $2
	`, accountID, limit)
	if err != nil {
		return nil, fmt.Errorf("player teams: %w", err)
	}
	defer rows.Close()

	var out []profiles.PlayerTeam
	for rows.Next() {
		var p profiles.PlayerTeam
		if err := rows.Scan(&p.TeamID, &p.Games, &p.Wins, &p.LastPlayed, &p.LastPatchID); err != nil {
			return nil, fmt.Errorf("scan player teams: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// RosterComfortAvgBatch returns average player comfort (avg wr_shrunk) across the roster per hero.
func (r *PGRepository) RosterComfortAvgBatch(ctx context.Context, roster []domain.AccountID, heroes []domain.HeroID) (map[domain.HeroID]float64, error) {
	out := make(map[domain.HeroID]float64, len(heroes))
	for _, h := range heroes {
		out[h] = 0
	}
	if len(roster) == 0 || len(heroes) == 0 {
		return out, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT hero_id, AVG(shrunk_wr)
		FROM analytics.mv_player_hero_profile
		WHERE account_id = ANY($1) AND hero_id = ANY($2)
		GROUP BY hero_id
	`, accountIDsToInt64(roster), heroIDsToInt16(heroes))
	if err != nil {
		return nil, fmt.Errorf("roster comfort avg batch: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var heroID int16
		var avg float64
		if err := rows.Scan(&heroID, &avg); err != nil {
			return nil, fmt.Errorf("scan roster comfort: %w", err)
		}
		out[domain.HeroID(heroID)] = avg
	}
	return out, rows.Err()
}
