package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/user-for-download/dota2-analysis/go-analysis/internal/domain"
	"github.com/user-for-download/dota2-analysis/go-analysis/internal/profiles"
)

// PGRepository implements profiles.Repository using Postgres materialized views.
type PGRepository struct {
	db *pgxpool.Pool
}

// NewPGRepository creates a new PGRepository.
func NewPGRepository(db *pgxpool.Pool) *PGRepository {
	return &PGRepository{db: db}
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

// ─── Helpers ───────────────────────────────────────────────

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
