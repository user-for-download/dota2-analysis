package postgres

import (
	"context"
	"fmt"

	"github.com/user-for-download/dota2-analysis/go-analysis/internal/domain"
	"github.com/user-for-download/dota2-analysis/go-analysis/internal/profiles"
)

// HeroSynergies returns hero synergy partners for a given hero.
func (r *PGRepository) HeroSynergies(ctx context.Context, heroID domain.HeroID, minGames, limit int) ([]profiles.HeroPair, error) {
	// mv_hero_synergy stores only one row per unordered pair (hero_a < hero_b).
	// Search both (hero_a = needle, hero_b = partner) and (hero_b = needle, hero_a = partner).
	rows, err := r.db.Query(ctx, `
		SELECT hero_b, hero_b_name, games, wins, shrunk_wr
		FROM analytics.mv_hero_synergy
		WHERE hero_a = $1 AND games >= $2
		UNION ALL
		SELECT hero_a, hero_a_name, games, wins, shrunk_wr
		FROM analytics.mv_hero_synergy
		WHERE hero_b = $1 AND games >= $2
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
			return nil, fmt.Errorf("scan hero synergies: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
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
			return nil, fmt.Errorf("scan hero counters: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
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
	// mv_hero_synergy stores only one row per unordered pair (hero_a < hero_b).
	// Search both (hero_a IN allies, hero_b IN candidates) and
	// (hero_a IN candidates, hero_b IN allies) to catch all pairs.
	rows, err := r.db.Query(ctx, `
		SELECT
			CASE WHEN hero_a = ANY($2) THEN hero_a ELSE hero_b END AS candidate_id,
			AVG(shrunk_wr)
		FROM analytics.mv_hero_synergy
		WHERE (hero_a = ANY($1) AND hero_b = ANY($2))
		   OR (hero_b = ANY($1) AND hero_a = ANY($2))
		GROUP BY 1
	`, heroIDsToInt16(allies), heroIDsToInt16(candidates))
	if err != nil {
		return nil, fmt.Errorf("synergy avg batch: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var heroID int16
		var avg float64
		if err := rows.Scan(&heroID, &avg); err != nil {
			return nil, fmt.Errorf("scan synergy avg: %w", err)
		}
		out[domain.HeroID(heroID)] = avg
	}
	return out, rows.Err()
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
			return nil, fmt.Errorf("scan counter avg: %w", err)
		}
		out[domain.HeroID(heroID)] = avg
	}
	return out, rows.Err()
}
