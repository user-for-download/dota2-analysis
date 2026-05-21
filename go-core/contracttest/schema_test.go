// Package contracttest verifies the canonical SQL migration sequence against
// a real Postgres instance. These tests require a reachable Postgres database
// and are skipped on short test runs (go test -short).
//
// Run them explicitly with a Postgres DSN:
//
//	POSTGRES_TEST_DSN="postgres://..." go test -v ./contracttest/
package contracttest

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/user-for-download/go-dota2-core/migrator"
	"github.com/user-for-download/go-dota2-core/schema"
)

// TestSchemaInvariants applies all embedded migrations to a real Postgres
// and verifies critical schema contracts that downstream projects depend on.
func TestSchemaInvariants(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping contract test in short mode")
	}

	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN not set; skipping contract test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// Apply all embedded migrations.
	if err := migrator.Run(ctx, dsn, schema.Migrations, log); err != nil {
		t.Fatalf("migrator.Run: %v", err)
	}

	// Open a connection for assertions.
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	// ---- Assert critical table structure ----
	assertColumnType(t, db, "public", "matches", "match_id", "bigint")
	assertColumnType(t, db, "public", "matches", "start_time", "bigint")
	assertColumnType(t, db, "public", "matches", "patch_id", "integer")
	assertColumnType(t, db, "public", "matches", "radiant_win", "boolean")
	assertColumnType(t, db, "public", "matches", "duration", "integer")
	assertColumnType(t, db, "public", "matches", "leagueid", "integer")

	assertColumnType(t, db, "public", "heroes", "id", "smallint")
	assertColumnType(t, db, "public", "heroes", "name", "text")

	assertColumnType(t, db, "public", "player_matches", "match_id", "bigint")
	assertColumnType(t, db, "public", "player_matches", "hero_id", "smallint")
	assertColumnType(t, db, "public", "player_matches", "account_id", "bigint")
	assertColumnType(t, db, "public", "player_matches", "patch_id", "integer")

	assertColumnType(t, db, "public", "picks_bans", "match_id", "bigint")
	assertColumnType(t, db, "public", "picks_bans", "hero_id", "smallint")
	assertColumnType(t, db, "public", "picks_bans", "is_pick", "boolean")

	assertColumnType(t, db, "public", "teams", "team_id", "bigint")

	// ---- Assert schema_migrations table exists and has records ----
	assertTableExists(t, db, "public", "schema_migrations")
	var migrationCount int
	err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM public.schema_migrations`).Scan(&migrationCount)
	if err != nil {
		t.Fatalf("count schema_migrations: %v", err)
	}
	if migrationCount < 2 {
		t.Errorf("expected at least 2 migration records, got %d", migrationCount)
	}

	// ---- Assert analytics schema matviews exist ----
	assertTableExists(t, db, "analytics", "mv_team_hero_profile")
	assertTableExists(t, db, "analytics", "mv_hero_synergy")
	assertTableExists(t, db, "analytics", "mv_hero_counter")
	assertTableExists(t, db, "analytics", "mv_player_team_history")
	assertTableExists(t, db, "analytics", "mv_player_hero_profile")
	assertTableExists(t, db, "analytics", "feature_snapshots_player_hero")
	assertTableExists(t, db, "analytics", "featurizer_runs")

	t.Logf("schema contract verified: %d migrations applied, all invariants pass", migrationCount)
}

func assertColumnType(t *testing.T, db *sql.DB, schema, table, column, wantType string) {
	t.Helper()
	var dataType string
	q := `
		SELECT data_type
		FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = $2 AND column_name = $3
	`
	err := db.QueryRow(q, schema, table, column).Scan(&dataType)
	if err != nil {
		t.Errorf("column %s.%s.%s: not found (%v)", schema, table, column, err)
		return
	}
	if !typeMatch(dataType, wantType) {
		t.Errorf("column %s.%s.%s: type = %s, want %s", schema, table, column, dataType, wantType)
	}
}

func assertTableExists(t *testing.T, db *sql.DB, schema, table string) {
	t.Helper()
	var exists bool
	q := `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = $1 AND table_name = $2
		)
	`
	err := db.QueryRow(q, schema, table).Scan(&exists)
	if err != nil {
		t.Errorf("check table %s.%s: %v", schema, table, err)
		return
	}
	if !exists {
		t.Errorf("table/view %s.%s does not exist", schema, table)
	}
}

// typeMatch handles PostgreSQL aliases (e.g. integer ↔ int4, bigint ↔ int8).
func typeMatch(got, want string) bool {
	if got == want {
		return true
	}
	// PG type aliases
	alias := map[string]string{
		"integer":  "int4",
		"smallint": "int2",
		"bigint":   "int8",
		"boolean":  "bool",
		"real":     "float4",
		"double precision": "float8",
		"character varying": "varchar",
	}
	if a, ok := alias[got]; ok {
		return a == want || a == got
	}
	if a, ok := alias[want]; ok {
		return a == got
	}
	return false
}
