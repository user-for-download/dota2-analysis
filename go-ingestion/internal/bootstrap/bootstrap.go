package bootstrap

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	coreconfig "github.com/user-for-download/dota2-analysis/go-core/config"
	corebootstrap "github.com/user-for-download/dota2-analysis/go-core/bootstrap"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/proxy"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/proxy/redisproxy"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/storage/matchstore"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/storage/pgclient"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/storage/refdatastore"
)

func ProxyPool(rdb *goredis.Client, cfg proxy.Config, log *slog.Logger) (proxy.Pool, error) {
	pool, err := redisproxy.New(rdb, redisproxy.Config{
		KeyPrefix: cfg.KeyPrefix,
		RateLimit: proxy.RateLimit{
			RatePerSec: cfg.RateLimitPerSec,
			Burst:     cfg.RateLimitBurst,
			Window:    cfg.RateLimitWindow,
		},
		Ranking: proxy.Ranking{
			InitialWeight:  cfg.RankingInitial,
			SuccessBoost:  cfg.RankingSuccess,
			FailurePenalty: cfg.RankingFailure,
		},
		MaxFailures: cfg.MaxFailures,
		Logger: log,
	})
	if err != nil {
		return nil, fmt.Errorf("proxy pool: %w", err)
	}
	return pool, nil
}

// Postgres opens a pgxpool with OTel tracing.
// Delegates to go-dota2-core/bootstrap.Postgres.
func Postgres(ctx context.Context, cfg coreconfig.PostgresConfig, log *slog.Logger) (*pgxpool.Pool, error) {
	return corebootstrap.Postgres(ctx, cfg, log)
}

// NewLogger wraps a slog.Handler with OTel trace ID injection.
// Delegates to go-dota2-core/bootstrap.NewLogger.
func NewLogger(h slog.Handler) *slog.Logger {
	return corebootstrap.NewLogger(h)
}

// NewLoggerFromEnv creates a JSON logger that respects the LOG_LEVEL env var.
// Defaults to INFO if LOG_LEVEL is unset or invalid.
func NewLoggerFromEnv() *slog.Logger {
	return corebootstrap.NewLoggerFromEnv()
}

// InitTelemetry sets up OTel tracing.
// Delegates to go-dota2-core/bootstrap.InitTelemetry.
func InitTelemetry(ctx context.Context, serviceName string, endpoint string, sampleRate float64) (func(context.Context) error, error) {
	return corebootstrap.InitTelemetry(ctx, serviceName, endpoint, sampleRate)
}

func MatchWriter(db *pgxpool.Pool, log *slog.Logger) matchstore.MatchWriter {
	return pgclient.NewStores(db, log).Matches
}

func ReferenceWriter(db *pgxpool.Pool, log *slog.Logger) refdatastore.RefDataWriter {
	return pgclient.NewStores(db, log).RefData
}