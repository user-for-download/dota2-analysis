// Package migrator provides a re-usable embedded-SQL migration runner.
//
// It discovers .sql files matching the pattern {version}_{name}.sql
// from an fs.FS (typically schema.Migrations), tracks applied versions
// in a schema_migrations table, and executes pending migrations in
// version order within a dedicated transaction per migration.
//
// Usage:
//
//	import (
//	    "github.com/user-for-download/dota2-analysis/go-core/migrator"
//	    "github.com/user-for-download/dota2-analysis/go-core/schema"
//	)
//
//	if err := migrator.Run(ctx, dsn, schema.Migrations, log); err != nil {
//	    log.Error("migration failed", "err", err)
//	    os.Exit(1)
//	}
package migrator

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

var filenameRe = regexp.MustCompile(`^(\d+)_[A-Za-z0-9_\-]+\.sql$`)

type migration struct {
	version  int
	filename string
	sql      string
}

// Run applies all pending migrations from fsys in lexicographic version order.
//
// It connects to the database at dsn, creates the schema_migrations tracking
// table if it does not exist, acquires an advisory lock (13625) to prevent
// concurrent migrators, and applies each un-applied migration in a transaction.
func Run(ctx context.Context, dsn string, fsys fs.FS, log *slog.Logger) error {
	db, err := openWithRetry(ctx, dsn, 30*time.Second, log)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire dedicated connection: %w", err)
	}
	defer func() {
		if cerr := conn.Close(); cerr != nil {
			log.Warn("failed to close connection", "err", cerr)
		}
	}()

	locked, err := tryAdvisoryLock(ctx, conn)
	if err != nil {
		return fmt.Errorf("advisory lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("another migrator instance is already running (lock held)")
	}

	if err := ensureTable(ctx, conn); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}

	migs, err := loadMigrations(fsys)
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}
	log.Info("migrations discovered", "count", len(migs))

	applied, err := loadApplied(ctx, conn)
	if err != nil {
		return fmt.Errorf("load applied: %w", err)
	}

	pending := 0
	for _, m := range migs {
		if _, ok := applied[m.version]; ok {
			log.Debug("skip (applied)", "version", m.version, "file", m.filename)
			continue
		}
		log.Info("applying", "version", m.version, "file", m.filename)
		if err := apply(ctx, conn, m); err != nil {
			return fmt.Errorf("apply %s (version %d): %w", m.filename, m.version, err)
		}
		pending++
	}

	log.Info("migration complete", "applied", pending, "total", len(migs))
	return nil
}

func tryAdvisoryLock(ctx context.Context, conn *sql.Conn) (bool, error) {
	var locked bool
	err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock(13625)").Scan(&locked)
	return locked, err
}

func openWithRetry(ctx context.Context, dsn string, max time.Duration, log *slog.Logger) (*sql.DB, error) {
	deadline := time.Now().Add(max)
	var lastErr error
	for {
		db, err := sql.Open("pgx", dsn)
		if err == nil {
			if err = db.PingContext(ctx); err == nil {
				return db, nil
			}
			db.Close()
			lastErr = err
		} else {
			lastErr = err
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("db not reachable after %s: %w", max, lastErr)
		}
		log.Warn("db not ready; retrying", "err", lastErr)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

func ensureTable(ctx context.Context, conn *sql.Conn) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    INT PRIMARY KEY,
    filename   TEXT NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);`
	_, err := conn.ExecContext(ctx, ddl)
	return err
}

func loadMigrations(fsys fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("read dir: %w", err)
	}

	var out []migration
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		match := filenameRe.FindStringSubmatch(e.Name())
		if match == nil {
			continue
		}
		v, err := strconv.Atoi(match[1])
		if err != nil {
			return nil, fmt.Errorf("bad version in %q: %w", e.Name(), err)
		}
		b, err := fs.ReadFile(fsys, e.Name())
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", e.Name(), err)
		}
		out = append(out, migration{version: v, filename: e.Name(), sql: string(b)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })

	for i := 1; i < len(out); i++ {
		if out[i].version == out[i-1].version {
			return nil, fmt.Errorf("duplicate migration version %d: %s and %s",
				out[i].version, out[i-1].filename, out[i].filename)
		}
	}
	return out, nil
}

func loadApplied(ctx context.Context, conn *sql.Conn) (map[int]string, error) {
	rows, err := conn.QueryContext(ctx, `SELECT version, filename FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]string{}
	for rows.Next() {
		var v int
		var f string
		if err := rows.Scan(&v, &f); err != nil {
			return nil, err
		}
		out[v] = f
	}
	return out, rows.Err()
}

func apply(ctx context.Context, conn *sql.Conn, m migration) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, m.sql); err != nil {
		return fmt.Errorf("exec %s: %w", m.filename, err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO schema_migrations (version, filename, applied_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (version) DO NOTHING
	`, m.version, m.filename)
	if err != nil {
		return fmt.Errorf("record %s: %w", m.filename, err)
	}
	return tx.Commit()
}
