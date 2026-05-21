package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/user-for-download/go-dota2-analysis/internal/domain"
	"github.com/user-for-download/go-dota2-analysis/internal/profiles"
)

// PGRepository implements profiles.Repository using Postgres materialized views.
type PGRepository struct {
	db *pgxpool.Pool
}

// NewPGRepository creates a new PGRepository.
func NewPGRepository(db *pgxpool.Pool) *PGRepository {
	return &PGRepository{db: db}
}

// HeroSynergies returns hero synergy partners for a given hero.
func (r *PGRepository) HeroSynergies(ctx context.Context, heroID domain.HeroID, minGames, limit int) ([]profiles.HeroPair, error) {
	rows, err := r.db.Query(ctx, `
		SELECT hero_b, hero_b_name, games, wins, shrunk_wr
		FROM analytics.mv_hero_synergy
		WHERE hero_a = $1 AND games >= $2
		ORDER BY shrunk_wr DESC
		LIMIT $3
	`, heroID, minGames, limit)
	if err != nil {
		return nil, fmt.Errorf("hero synergies: %w", err)
	}
	defer rows.Close()

	var out []profiles.HeroPair
	for rows.Next() {
		var p profiles.HeroPair
		if err := rows.Scan(&p.HeroID, &p.HeroName, &p.Games, &p.Wins, &p.WRShrunk); err != nil {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

// HeroCounters returns heroes that counter a given hero.
func (r *PGRepository) HeroCounters(ctx context.Context, heroID domain.HeroID, minGames, limit int) ([]profiles.HeroPair, error) {
	rows, err := r.db.Query(ctx, `
		SELECT hero_b, hero_b_name, games, wins, shrunk_wr
		FROM analytics.mv_hero_counter
		WHERE hero_a = $1 AND games >= $2
		ORDER BY shrunk_wr ASC
		LIMIT $3
	`, heroID, minGames, limit)
	if err != nil {
		return nil, fmt.Errorf("hero counters: %w", err)
	}
	defer rows.Close()

	var out []profiles.HeroPair
	for rows.Next() {
		var p profiles.HeroPair
		if err := rows.Scan(&p.HeroID, &p.HeroName, &p.Games, &p.Wins, &p.WRShrunk); err != nil {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

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
			continue
		}
		out = append(out, p)
	}
	return out, nil
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
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

// FeaturizerStatus returns the featurizer's health status.
func (r *PGRepository) FeaturizerStatus(ctx context.Context) (profiles.FeaturizerStatus, error) {
	var status profiles.FeaturizerStatus
	err := r.db.QueryRow(ctx, `
		SELECT GREATEST(
		    COALESCE(last_mv_refresh_at, '1970-01-01'),
		    COALESCE(last_snapshot_at, '1970-01-01')
		)
		FROM analytics.featurizer_runs
		WHERE id = 1
	`).Scan(&status.LastSuccessful)
	return status, err
}

// ──────────────────────────────────────────────
// Feature-source batch lookups
// ──────────────────────────────────────────────

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

// SynergyAvgBatch returns average synergy WR between ally picks and candidates.
func (r *PGRepository) SynergyAvgBatch(ctx context.Context, allies, candidates []domain.HeroID) (map[domain.HeroID]float64, error) {
	out := make(map[domain.HeroID]float64, len(candidates))
	for _, h := range candidates {
		out[h] = 0
	}
	if len(allies) == 0 || len(candidates) == 0 {
		return out, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT hero_b, AVG(shrunk_wr)
		FROM analytics.mv_hero_synergy
		WHERE hero_a = ANY($1) AND hero_b = ANY($2)
		GROUP BY hero_b
	`, heroIDsToInt16(allies), heroIDsToInt16(candidates))
	if err != nil {
		return nil, fmt.Errorf("synergy avg batch: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var heroID int16
		var avg float64
		if err := rows.Scan(&heroID, &avg); err != nil {
			continue
		}
		out[domain.HeroID(heroID)] = avg
	}
	return out, nil
}

// CounterAvgBatch returns average counter WR of candidates against enemies.
func (r *PGRepository) CounterAvgBatch(ctx context.Context, candidates, enemies []domain.HeroID) (map[domain.HeroID]float64, error) {
	out := make(map[domain.HeroID]float64, len(candidates))
	for _, h := range candidates {
		out[h] = 0
	}
	if len(enemies) == 0 || len(candidates) == 0 {
		return out, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT hero_a, AVG(shrunk_wr)
		FROM analytics.mv_hero_counter
		WHERE hero_a = ANY($1) AND hero_b = ANY($2)
		GROUP BY hero_a
	`, heroIDsToInt16(candidates), heroIDsToInt16(enemies))
	if err != nil {
		return nil, fmt.Errorf("counter avg batch: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var heroID int16
		var avg float64
		if err := rows.Scan(&heroID, &avg); err != nil {
			continue
		}
		out[domain.HeroID(heroID)] = avg
	}
	return out, nil
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
			continue
		}
		out[domain.HeroID(heroID)] = avg
	}
	return out, nil
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

// ──────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────

func heroIDsToInt16(hs []domain.HeroID) []int16 {
	out := make([]int16, len(hs))
	for i, h := range hs {
		out[i] = int16(h)
	}
	return out
}

func accountIDsToInt64(ids []domain.AccountID) []int64 {
	out := make([]int64, len(ids))
	for i, id := range ids {
		out[i] = int64(id)
	}
	return out
}
