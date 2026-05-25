package postgres

import (
	"context"
	"fmt"

	"github.com/user-for-download/go-dota2-analysis/internal/domain"
	"github.com/user-for-download/go-dota2-analysis/internal/profiles"
)

// TeamHeroes returns a team's historical performance with each hero.
func (r *PGRepository) TeamHeroes(ctx context.Context, teamID domain.TeamID, minGames, limit int) ([]profiles.TeamHero, error) {
	rows, err := r.db.Query(ctx, `
		SELECT hero_id, hero_name, games, wins, shrunk_wr
		FROM analytics.mv_team_hero_profile
		WHERE team_id = $1 AND games >= $2
		ORDER BY games DESC
		LIMIT $3
	`, teamID, minGames, limit)
	if err != nil {
		return nil, fmt.Errorf("team heroes: %w", err)
	}
	defer rows.Close()

	var out []profiles.TeamHero
	for rows.Next() {
		var t profiles.TeamHero
		if err := rows.Scan(&t.HeroID, &t.HeroName, &t.Games, &t.Wins, &t.WRShrunk); err != nil {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

// TeamH2H returns head-to-head record between two teams.
func (r *PGRepository) TeamH2H(ctx context.Context, teamA, teamB domain.TeamID) (profiles.H2HRecord, error) {
	var rec profiles.H2HRecord
	err := r.db.QueryRow(ctx, `
		SELECT COUNT(*),
		       COALESCE(SUM(CASE WHEN
		           (tm.is_radiant AND m.radiant_win) OR
		           (NOT tm.is_radiant AND NOT m.radiant_win)
		       THEN 1 ELSE 0 END), 0)
		FROM public.team_matches tm
		JOIN public.matches m ON m.match_id = tm.match_id
		WHERE tm.team_id = $1
		  AND m.leagueid > 0
		  AND EXISTS (
		      SELECT 1 FROM public.team_matches tm2
		      WHERE tm2.match_id = m.match_id AND tm2.team_id = $2
		  )
	`, teamA, teamB).Scan(&rec.Games, &rec.TeamAWins)
	rec.TeamBWins = rec.Games - rec.TeamAWins
	return rec, err
}

// TeamHeroStatsBatch returns games and wr_shrunk for each hero for a team.
func (r *PGRepository) TeamHeroStatsBatch(ctx context.Context, teamID domain.TeamID, heroes []domain.HeroID) (map[domain.HeroID]profiles.TeamHeroStats, error) {
	result := make(map[domain.HeroID]profiles.TeamHeroStats, len(heroes))
	for _, h := range heroes {
		result[h] = profiles.TeamHeroStats{Games: 0, WRShrunk: 0}
	}
	if len(heroes) == 0 {
		return result, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT hero_id, games, shrunk_wr
		FROM analytics.mv_team_hero_profile
		WHERE team_id = $1 AND hero_id = ANY($2)
	`, teamID, heroIDsToInt16(heroes))
	if err != nil {
		return result, nil // missing data is OK
	}
	defer rows.Close()
	for rows.Next() {
		var heroID int16
		var s profiles.TeamHeroStats
		if err := rows.Scan(&heroID, &s.Games, &s.WRShrunk); err != nil {
			continue
		}
		result[domain.HeroID(heroID)] = s
	}
	return result, nil
}

// StarThreatBatch returns the opponent team's WR for each hero (min 3 games).
func (r *PGRepository) StarThreatBatch(ctx context.Context, themTeamID domain.TeamID, heroes []domain.HeroID, minGames int) (map[domain.HeroID]float64, error) {
	out := make(map[domain.HeroID]float64, len(heroes))
	for _, h := range heroes {
		out[h] = 0
	}
	if len(heroes) == 0 {
		return out, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT hero_id, shrunk_wr
		FROM analytics.mv_team_hero_profile
		WHERE team_id = $1 AND hero_id = ANY($2) AND games >= $3
	`, themTeamID, heroIDsToInt16(heroes), minGames)
	if err != nil {
		return out, nil // missing data is OK
	}
	defer rows.Close()
	for rows.Next() {
		var heroID int16
		var wr float64
		if err := rows.Scan(&heroID, &wr); err != nil {
			continue
		}
		out[domain.HeroID(heroID)] = wr
	}
	return out, nil
}
