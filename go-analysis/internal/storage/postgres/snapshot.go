package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SnapshotPlayerHero inserts a point-in-time snapshot of mv_player_hero_profile
// and records the run in featurizer_runs.
func SnapshotPlayerHero(ctx context.Context, db *pgxpool.Pool, now time.Time) error {
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
		(SELECT MAX(patch_id) - 2 FROM public.matches),  -- -2: pad to exclude current + most-recent patch (still settling)
		(SELECT MAX(patch_id) FROM public.matches)
	FROM analytics.mv_player_hero_profile
	WHERE games >= 1
	`
	_, err := db.Exec(ctx, snapshotSQL, now)
	if err != nil {
		return fmt.Errorf("insert snapshot: %w", err)
	}

	const runSQL = `
	INSERT INTO analytics.featurizer_runs (id, last_snapshot_at, snapshot_status)
	VALUES (1, $1, 'success')
	ON CONFLICT (id) DO UPDATE
	SET last_snapshot_at = EXCLUDED.last_snapshot_at,
	    snapshot_status  = EXCLUDED.snapshot_status
	`
	_, err = db.Exec(ctx, runSQL, now)
	if err != nil {
		return fmt.Errorf("update featurizer_runs: %w", err)
	}

	return nil
}
