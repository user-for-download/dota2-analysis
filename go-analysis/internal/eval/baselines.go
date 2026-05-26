package eval

import (
	"context"
	"fmt"
	"math/rand"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/user-for-download/dota2-analysis/go-analysis/internal/domain"
)

// UniformRandomBaseline assigns random scores to candidates.
type UniformRandomBaseline struct{}

// Score returns randomly scored candidates (uniform [0,1)).
func (UniformRandomBaseline) Score(candidates []domain.HeroID) []domain.Score {
	scores := make([]domain.Score, len(candidates))
	for i, h := range candidates {
		scores[i] = domain.Score{Hero: h, Value: rand.Float64()}
	}
	return scores
}

// PickFrequencyBaseline weights candidates by their global pick frequency
// within a specific patch.
type PickFrequencyBaseline struct {
	Freqs map[domain.HeroID]float64
}

// NewPickFrequencyBaseline loads hero pick frequencies for a given patch.
func NewPickFrequencyBaseline(ctx context.Context, db *pgxpool.Pool, patchID int32) (*PickFrequencyBaseline, error) {
	rows, err := db.Query(ctx, `
		SELECT pb.hero_id, COUNT(*) as picks
		FROM public.picks_bans pb
		JOIN public.matches m ON m.match_id = pb.match_id
		WHERE pb.is_pick = true AND m.patch_id = $1 AND m.leagueid > 0
		GROUP BY pb.hero_id
	`, patchID)
	if err != nil {
		return nil, fmt.Errorf("query pick frequencies: %w", err)
	}
	defer rows.Close()

	freqs := make(map[domain.HeroID]float64)
	var total float64
	for rows.Next() {
		var heroID int16
		var picks int64
		if err := rows.Scan(&heroID, &picks); err != nil {
			return nil, fmt.Errorf("scan frequency: %w", err)
		}
		freqs[domain.HeroID(heroID)] = float64(picks)
		total += float64(picks)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate frequencies: %w", err)
	}

	// Normalize to probabilities.
	if total > 0 {
		for h := range freqs {
			freqs[h] /= total
		}
	}

	return &PickFrequencyBaseline{Freqs: freqs}, nil
}

// Score returns candidates scored by their normalized pick frequency.
// Heroes with no observed picks get a score of 0.
func (b *PickFrequencyBaseline) Score(candidates []domain.HeroID) []domain.Score {
	scores := make([]domain.Score, len(candidates))
	for i, h := range candidates {
		scores[i] = domain.Score{Hero: h, Value: b.Freqs[h]}
	}
	return scores
}

// PlayerComfortBaseline weights candidates by the best player comfort on the roster.
type PlayerComfortBaseline struct {
	db *pgxpool.Pool
}

// NewPlayerComfortBaseline creates a player-aware baseline that scores heroes
// by the highest win-rate shrunk value among roster players on that hero.
func NewPlayerComfortBaseline(db *pgxpool.Pool) *PlayerComfortBaseline {
	return &PlayerComfortBaseline{db: db}
}

// Score implements Baseline (no roster context — returns uniform scores).
func (b *PlayerComfortBaseline) Score(candidates []domain.HeroID) []domain.Score {
	return uniformScores(candidates)
}

// ScoreWithRoster returns candidates scored by the best player comfort (wr_shrunk) on the roster.
// Falls back to uniform scores if no player data is available.
func (b *PlayerComfortBaseline) ScoreWithRoster(ctx context.Context, roster []domain.AccountID, candidates []domain.HeroID) ([]domain.Score, error) {
	if len(candidates) == 0 {
		return nil, nil
	}

	heroIDs := make([]int16, len(candidates))
	for i, h := range candidates {
		heroIDs[i] = int16(h)
	}

	// Get best player comfort for each hero across roster.
	rows, err := b.db.Query(ctx, `
		SELECT hero_id, MAX(shrunk_wr) as best_wr
		FROM analytics.mv_player_hero_profile
		WHERE account_id = ANY($1) AND hero_id = ANY($2)
		GROUP BY hero_id
	`, accountIDs(roster), heroIDs)
	if err != nil {
		// Fallback to uniform random if no player data.
		return uniformScores(candidates), nil
	}
	defer rows.Close()

	scores := make(map[domain.HeroID]float64, len(candidates))
	for _, h := range candidates {
		scores[h] = 0.5 // default
	}

	for rows.Next() {
		var heroID int16
		var wr float64
		if err := rows.Scan(&heroID, &wr); err != nil {
			return nil, fmt.Errorf("scan comfort: %w", err)
		}
		scores[domain.HeroID(heroID)] = wr
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate comfort: %w", err)
	}

	out := make([]domain.Score, len(candidates))
	for i, h := range candidates {
		out[i] = domain.Score{Hero: h, Value: scores[h]}
	}
	return out, nil
}

// accountIDs converts domain.AccountID slice to int64 slice for pgx ANY().
func accountIDs(ids []domain.AccountID) []int64 {
	out := make([]int64, len(ids))
	for i, id := range ids {
		out[i] = int64(id)
	}
	return out
}

// uniformScores returns equal 0.5 scores for all candidates.
func uniformScores(candidates []domain.HeroID) []domain.Score {
	out := make([]domain.Score, len(candidates))
	for i, h := range candidates {
		out[i] = domain.Score{Hero: h, Value: 0.5}
	}
	return out
}
