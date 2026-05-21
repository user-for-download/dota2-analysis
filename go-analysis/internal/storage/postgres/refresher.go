package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// mvList is the ordered list of materialized views to refresh.
var mvList = []string{
	"mv_team_hero_profile",
	"mv_hero_synergy",
	"mv_hero_counter",
	"mv_player_team_history",
	"mv_player_hero_profile",
}

// RefreshAll refreshes all analytics materialized views in order.
func RefreshAll(ctx context.Context, db *pgxpool.Pool) error {
	for _, view := range mvList {
		if err := RefreshMV(ctx, db, view); err != nil {
			return err
		}
	}
	return nil
}

// RefreshMV refreshes a single materialized view.
// Uses CONCURRENTLY if the view is already populated (non-blocking),
// otherwise does a full refresh (ACCESS EXCLUSIVE lock, required for first population).
func RefreshMV(ctx context.Context, db *pgxpool.Pool, view string) error {
	populated, err := isMVPopulated(ctx, db, view)
	if err != nil {
		return fmt.Errorf("check population %s: %w", view, err)
	}

	// PostgreSQL syntax: REFRESH MATERIALIZED VIEW [CONCURRENTLY] name
	viewIdent := pgx.Identifier{"analytics", view}.Sanitize()
	query := "REFRESH MATERIALIZED VIEW"
	if populated {
		query += " CONCURRENTLY"
	}
	query += " " + viewIdent

	_, err = db.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("refresh %s: %w", view, err)
	}
	return nil
}

// isMVPopulated checks whether a materialized view has been populated.
// Returns false for views created WITH NO DATA or never refreshed.
func isMVPopulated(ctx context.Context, db *pgxpool.Pool, view string) (bool, error) {
	var populated bool
	err := db.QueryRow(ctx, `
		SELECT ispopulated FROM pg_matviews
		WHERE schemaname = 'analytics' AND matviewname = $1
	`, view).Scan(&populated)
	if err != nil {
		return false, err
	}
	return populated, nil
}
