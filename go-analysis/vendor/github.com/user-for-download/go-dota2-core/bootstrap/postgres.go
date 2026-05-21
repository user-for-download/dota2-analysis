// Package bootstrap provides re-usable initialization helpers
// (Postgres, logging, telemetry) shared across go-dota2-* projects.
package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/user-for-download/go-dota2-core/config"
)

// Postgres opens a pgxpool connection with OTel tracing and connection-pool safety.
//
// The pool uses DISCARD TEMP on connection release (not DISCARD ALL) to reset
// session state without losing prepared statements or session settings that
// other pool consumers may rely on.
func Postgres(ctx context.Context, cfg config.PostgresConfig, log *slog.Logger) (*pgxpool.Pool, error) {
	if cfg.DSN == "" {
		return nil, fmt.Errorf("postgres: DSN is required")
	}

	pcfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}

	// Attach OTel tracing to every connection.
	pcfg.ConnConfig.Tracer = otelpgx.NewTracer()

	// Reset temp objects on return to pool — lighter than DISCARD ALL.
	pcfg.AfterRelease = func(conn *pgx.Conn) bool {
		_, _ = conn.Exec(context.Background(), "DISCARD TEMP")
		return true
	}

	if cfg.MaxOpenConns > 0 {
		pcfg.MaxConns = int32(cfg.MaxOpenConns)
	}
	if cfg.ConnMaxLifetime > 0 {
		pcfg.MaxConnLifetime = cfg.ConnMaxLifetime
	}
	if cfg.ConnMaxIdleTime > 0 {
		pcfg.MaxConnIdleTime = cfg.ConnMaxIdleTime
	}

	pool, err := pgxpool.NewWithConfig(ctx, pcfg)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}

	// Verify connectivity.
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}

	log.Info("postgres connected")
	return pool, nil
}
