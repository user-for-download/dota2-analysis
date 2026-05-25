package bootstrap

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	coreconfig "github.com/user-for-download/go-dota2-core/config"
	corebootstrap "github.com/user-for-download/go-dota2-core/bootstrap"

	"github.com/user-for-download/go-dota2/internal/config"
	"github.com/user-for-download/go-dota2/internal/dedup"
	"github.com/user-for-download/go-dota2/internal/dedup/redisseen"
	"github.com/user-for-download/go-dota2/internal/metrics"
	"github.com/user-for-download/go-dota2/internal/metrics/otelmetrics"
	"github.com/user-for-download/go-dota2/internal/payload"
	"github.com/user-for-download/go-dota2/internal/payload/redisstore"
	"github.com/user-for-download/go-dota2/internal/proxy"
	"github.com/user-for-download/go-dota2/internal/proxy/redisproxy"
	"github.com/user-for-download/go-dota2/internal/queue"
	"github.com/user-for-download/go-dota2/internal/queue/redisstreams"
	"github.com/user-for-download/go-dota2/internal/storage/matchstore"
	"github.com/user-for-download/go-dota2/internal/storage/pgclient"
	storageredis "github.com/user-for-download/go-dota2/internal/storage/redis"

	"github.com/user-for-download/go-dota2/internal/storage/refdatastore"
)

type Deps struct {
	Redis   *storageredis.Client
	Metrics metrics.Sink
	Log     *slog.Logger
}

func (d *Deps) Close() error {
	if d.Redis != nil {
		return d.Redis.Close()
	}
	return nil
}

func Core(cfg *config.Config, log *slog.Logger) (*Deps, error) {
	rc, err := RedisClient(cfg.Redis, log)
	if err != nil {
		return nil, err
	}

	sink, err := otelmetrics.New()
	if err != nil {
		rc.Close()
		return nil, fmt.Errorf("init otelmetrics: %w", err)
	}

	return &Deps{Redis: rc, Metrics: sink, Log: log}, nil
}

func RedisClient(cfg config.RedisConfig, log *slog.Logger) (*storageredis.Client, error) {
	if len(cfg.Addrs) == 0 {
		return nil, fmt.Errorf("redis: no addresses")
	}
	rc, err := storageredis.New(storageredis.Config{
		Addrs:           cfg.Addrs,
		Password:       cfg.Password,
		DB:             cfg.DB,
		PoolSize:       cfg.PoolSize,
		MinIdleConns:    cfg.MinIdleConns,
		MaxActiveConns: cfg.MaxActiveConns,
		ConnMaxLifetime: cfg.ConnMaxLifetime,
		ConnMaxIdleTime: cfg.ConnMaxIdleTime,
		DialTimeout:    cfg.DialTimeout,
		ReadTimeout:   cfg.ReadTimeout,
		WriteTimeout:  cfg.WriteTimeout,
		ReadOnly:      cfg.ReadOnly,
	})
	if err != nil {
		return nil, fmt.Errorf("redis connect: %w", err)
	}
	return rc, nil
}

func ProxyPool(rdb *goredis.Client, cfg config.ProxyConfig, log *slog.Logger) (proxy.Pool, error) {
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

func PayloadStore(rdb *goredis.Client, cfg config.PayloadConfig) (payload.Store, error) {
	s, err := redisstore.New(rdb, redisstore.Config{
		KeyPrefix:  cfg.KeyPrefix,
		DefaultTTL: cfg.DefaultTTL,
	})
	if err != nil {
		return nil, fmt.Errorf("payload store: %w", err)
	}
	return s, nil
}

func DedupSeen(rdb *goredis.Client, cfg config.DedupConfig) (dedup.Seen, error) {
	s, err := redisseen.New(rdb, redisseen.Config{
		KeyPrefix:      cfg.KeyPrefix,
		TTL:            cfg.TTL,
		UseBloom:       cfg.UseBloom,
		BloomCapacity:  cfg.BloomCapacity,
		BloomErrorRate: cfg.BloomErrorRate,
	})
	if err != nil {
		return nil, fmt.Errorf("dedup seen: %w", err)
	}
	return s, nil
}

type QueueSpec struct {
	Stream    string
	DLQStream string
}

func Queue(rdb *goredis.Client, spec QueueSpec, cfg config.QueueConfig, log *slog.Logger) (queue.PubSub, error) {
	q, err := redisstreams.New(rdb, redisstreams.Config{
		Stream:      spec.Stream,
		DLQStream:   spec.DLQStream,
		Group:       cfg.Group,
		Consumer:    cfg.Consumer,
		MaxLen:      cfg.MaxLen,
		DeleteOnAck: cfg.DeleteOnAck,
		Policy: queue.RetryPolicy{
			MaxRetries: cfg.MaxRetries,
			MaxBackoff: cfg.MaxBackoff,
		},
		Logger: log,
	})
	if err != nil {
		return nil, fmt.Errorf("queue %s: %w", spec.Stream, err)
	}
	return q, nil
}

func FetchQueue(rdb *goredis.Client, cfg config.QueueConfig, log *slog.Logger) (queue.PubSub, error) {
	return Queue(rdb, QueueSpec{Stream: cfg.FetchStream, DLQStream: cfg.FetchDLQStream}, cfg, log)
}

func ParseQueue(rdb *goredis.Client, cfg config.QueueConfig, log *slog.Logger) (queue.PubSub, error) {
	return Queue(rdb, QueueSpec{Stream: cfg.ParseStream, DLQStream: cfg.ParseDLQStream}, cfg, log)
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
	return corebootstrap.Postgres(ctx, coreCfg, log)
}

// NewLogger wraps a slog.Handler with OTel trace ID injection.
// Delegates to go-dota2-core/bootstrap.NewLogger.
func NewLogger(h slog.Handler) *slog.Logger {
	return corebootstrap.NewLogger(h)
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