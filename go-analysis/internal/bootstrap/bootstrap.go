package bootstrap

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	corebootstrap "github.com/user-for-download/go-dota2-core/bootstrap"
	coreconfig "github.com/user-for-download/go-dota2-core/config"

	"github.com/user-for-download/go-dota2-analysis/internal/config"
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
