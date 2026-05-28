package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SnapshotPlayerHero inserts a point-in-time snapshot of mv_player_hero_profile
// and records the run in featurizer_runs, all in a single transaction.
func SnapshotPlayerHero(ctx context.Context, db *pgxpool.Pool, now time.Time) (err error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	const snapshotSQL = `
	INSERT INTO analytics.feature_snapshots_player_hero
	(snapshot_at, account_id, hero_id, games, wins, shrunk_wr, patch_min, patch_max)
	SELECT
		$1,
		account_id,
		hero_id,
		games,
		wins,
		shrunk_wr,
		p.max_patch - 2,  -- -2: pad to exclude current + most-recent patch (still settling)
		p.max_patch
	FROM analytics.mv_player_hero_profile
	CROSS JOIN (
		SELECT COALESCE(MAX(patch_id), 0) AS max_patch FROM public.matches
	) p
	WHERE games >= 1
	`
	if _, err = tx.Exec(ctx, snapshotSQL, now); err != nil {
		return fmt.Errorf("insert snapshot: %w", err)
	}

	const runSQL = `
	INSERT INTO analytics.featurizer_runs (id, last_snapshot_at, snapshot_status)
	VALUES (1, $1, 'success')
	ON CONFLICT (id) DO UPDATE
	SET last_snapshot_at = EXCLUDED.last_snapshot_at,
	    snapshot_status  = EXCLUDED.snapshot_status
	`
	if _, err = tx.Exec(ctx, runSQL, now); err != nil {
		return fmt.Errorf("update featurizer_runs: %w", err)
	}

	return tx.Commit(ctx)
}
