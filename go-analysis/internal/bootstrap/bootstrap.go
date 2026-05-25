package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	corebootstrap "github.com/user-for-download/dota2-analysis/go-core/bootstrap"
	coreconfig "github.com/user-for-download/dota2-analysis/go-core/config"

	"github.com/user-for-download/dota2-analysis/go-analysis/internal/config"
)

// Deps holds shared dependencies for analysis services.
type Deps struct {
	DB  *pgxpool.Pool
	Log *slog.Logger
}

func (d *Deps) Close() error {
	if d.DB != nil {
		d.DB.Close()
	}
	return nil
}

// Core initializes shared dependencies: Postgres pool and logger.
func Core(ctx context.Context, cfg *config.Config, log *slog.Logger) (*Deps, error) {
	db, err := Postgres(ctx, cfg.Postgres, log)
	if err != nil {
		return nil, err
	}
	return &Deps{DB: db, Log: log}, nil
}

// Postgres opens a pgxpool with OTel tracing.
// Delegates to go-dota2-core/bootstrap.Postgres.
func Postgres(ctx context.Context, cfg config.PostgresConfig, log *slog.Logger) (*pgxpool.Pool, error) {
	coreCfg := coreconfig.PostgresConfig{
		DSN:             cfg.DSN,
		MaxOpenConns:    cfg.MaxOpenConns,
		MaxIdleConns:    cfg.MaxIdleConns,
		ConnMaxLifetime: cfg.ConnMaxLifetime,
		ConnMaxIdleTime: cfg.ConnMaxIdleTime,
	}
	db, err := corebootstrap.Postgres(ctx, coreCfg, log)
	if err != nil {
		return nil, fmt.Errorf("postgres: %w", err)
	}
	return db, nil
}

// NewLogger wraps a slog.Handler with OTel trace ID injection.
// Delegates to go-dota2-core/bootstrap.NewLogger.
func NewLogger(h slog.Handler) *slog.Logger {
	return corebootstrap.NewLogger(h)
}

// InitTelemetry sets up OTel tracing. Returns a no-op shutdown if endpoint is empty.
// Delegates to go-dota2-core/bootstrap.InitTelemetry.
func InitTelemetry(ctx context.Context, serviceName string, endpoint string, sampleRate float64) (func(context.Context) error, error) {
	return corebootstrap.InitTelemetry(ctx, serviceName, endpoint, sampleRate)
}

// WaitForLaunchKey blocks until the specified key exists in analytics.launch_keys.
// This prevents read-side services from querying unpopulated materialized views.
func WaitForLaunchKey(ctx context.Context, db *pgxpool.Pool, key string, log *slog.Logger) error {
	log.Info("waiting for launch key", "key", key)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		var exists bool
		err := db.QueryRow(ctx, `
            SELECT EXISTS(SELECT 1 FROM analytics.launch_keys WHERE key = $1)
        `, key).Scan(&exists)

		if err == nil && exists {
			log.Info("launch key acquired", "key", key)
			return nil
		}

		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "42P01" {
				// undefined_table: migrator hasn't created it yet
				log.Debug("launch_keys table not found yet, waiting for migrator")
			} else {
				log.Debug("launch key check failed (will retry)", "err", err)
			}
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled waiting for launch key %q: %w", key, ctx.Err())
		case <-ticker.C:
		}
	}
}
