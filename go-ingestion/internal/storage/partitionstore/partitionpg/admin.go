package partitionpg

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/storage/partitionstore"
)

type Admin struct {
	db  *pgxpool.Pool
	log *slog.Logger
}

func NewAdmin(db *pgxpool.Pool, log *slog.Logger) *Admin {
	if log == nil {
		log = slog.Default()
	}
	return &Admin{db: db, log: log.With("component", "partitionpg")}
}

var _ partitionstore.PartitionAdmin = (*Admin)(nil)

func (a *Admin) EnsurePartitions(ctx context.Context, until time.Time) error {
	now := time.Now()
	if until.Before(now) {
		return nil
	}
	// Calculate how many quarters ahead we need to ensure.
	// Each quarter is ~3 months, so we round up to cover the full range.
	months := int(until.Sub(now).Hours() / (24 * 30))
	quarters := (months / 3) + 1
	if quarters < 1 {
		quarters = 1
	}
	_, err := a.db.Exec(ctx, "SELECT ensure_future_time_partitions(ARRAY['matches','player_matches','public_matches','player_timeseries'], $1)", quarters)
	return err
}