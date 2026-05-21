package featurize

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/user-for-download/go-dota2-analysis/internal/storage/postgres"
)

// Runner executes the featurizer loop: refresh MVs and record snapshots.
type Runner struct {
	db       *pgxpool.Pool
	interval time.Duration
	log      *slog.Logger
}

// NewRunner creates a new featurizer Runner.
func NewRunner(db *pgxpool.Pool, interval time.Duration, log *slog.Logger) *Runner {
	return &Runner{
		db:       db,
		interval: interval,
		log:      log,
	}
}

// Run executes the featurizer loop. Runs once immediately, then waits for interval.
func (r *Runner) Run(ctx context.Context) error {
	for {
		if err := r.runOnce(ctx); err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			r.log.Error("featurizer run failed", "err", err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(r.interval):
		}
	}
}

// runOnce performs a single refresh + snapshot cycle.
func (r *Runner) runOnce(ctx context.Context) error {
	r.log.Info("featurizer: starting refresh cycle")
	start := time.Now()

	if err := postgres.RefreshAll(ctx, r.db); err != nil {
		return err
	}

	now := time.Now().UTC()
	if err := postgres.SnapshotPlayerHero(ctx, r.db, now); err != nil {
		return err
	}

	r.log.Info("featurizer: refresh cycle complete", "duration", time.Since(start).Truncate(time.Millisecond))
	return nil
}
