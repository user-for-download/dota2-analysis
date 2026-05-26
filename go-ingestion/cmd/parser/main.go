package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	coreconfig "github.com/user-for-download/dota2-analysis/go-core/config"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/bootstrap"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/dedup/redisseen"
	metricstypes "github.com/user-for-download/dota2-analysis/go-ingestion/internal/metrics"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/metrics/otelmetrics"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/payload"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/payload/redisstore"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/queue"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/queue/redisstreams"
	storageredis "github.com/user-for-download/dota2-analysis/go-ingestion/internal/storage/redis"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/storage/pgclient"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/queue/middleware"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/resilience"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/worker/ingester"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/worker/parser"
)

func main() {
	log := bootstrap.NewLogger(slog.NewJSONHandler(os.Stdout, nil))

	// ── Load configs from domain packages ──────────────────────────
	redisCfg := storageredis.LoadConfig()
	queueCfg := queue.LoadConfig()
	payloadCfg := payload.LoadConfig()      // from internal/payload/config.go
	parserCfg := parser.LoadConfig()

	// ── Signal context ─────────────────────────────────────────────
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// ── Telemetry ──────────────────────────────────────────────────
	shutdownTelemetry, err := bootstrap.InitTelemetry(ctx, "go-dota2-parser",
		getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
		getEnvFloat("OTEL_SAMPLE_RATE", 1.0))
	if err != nil {
		log.Error("init telemetry", "err", err)
	} else if shutdownTelemetry != nil {
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = shutdownTelemetry(shutdownCtx)
		}()
	}

	// ── Redis client ───────────────────────────────────────────────
	rdb, err := storageredis.New(redisCfg)
	must(log, "redis", err)

	// ── Metrics ────────────────────────────────────────────────────
	sink, err := otelmetrics.New()
	must(log, "metrics", err)

	// ── Postgres ───────────────────────────────────────────────────
	pgCfg := loadPostgresConfig()
	db, err := bootstrap.WaitForPostgres(ctx, pgCfg, bootstrap.WaitConfig{
		Timeout:      0,
		PollInterval: 30 * time.Second,
	}, log)
	must(log, "postgres", err)
	defer db.Close()

	// ── Stores & Repos ─────────────────────────────────────────────
	stores := pgclient.NewStores(db, log)
	repo := stores.Matches // matchstore.MatchWriter (also MatchStore)

	// ── Queue (parser consumer) ────────────────────────────────────
	parseQ, err := redisstreams.New(rdb.Master(), redisstreams.Config{
		Stream:      queueCfg.ParseStream,
		DLQStream:   queueCfg.ParseDLQStream,
		Group:       queueCfg.Group,
		Consumer:    queueCfg.Consumer,
		MaxLen:      queueCfg.MaxLen,
		DeleteOnAck: queueCfg.DeleteOnAck,
		Policy: queue.RetryPolicy{
			MaxRetries: queueCfg.MaxRetries,
			MaxBackoff: queueCfg.MaxBackoff,
		},
		Logger: log,
	})
	must(log, "parse queue", err)

	// ── Payload Store ──────────────────────────────────────────────
	store, err := redisstore.New(rdb.Master(), redisstore.Config{
		KeyPrefix:  payloadCfg.KeyPrefix,
		DefaultTTL: payloadCfg.DefaultTTL,
	})
	must(log, "payload store", err)

	// ── Dedup Seen Set ─────────────────────────────────────────────
	dedupSeen, err := redisseen.New(rdb.Master(), redisseen.Config{
		KeyPrefix:      getEnv("DEDUP_KEY_PREFIX", "dota2:seen"),
		TTL:            getEnvDur("DEDUP_TTL", 24*time.Hour),
		UseBloom:       getEnvBool("DEDUP_USE_BLOOM", false),
		BloomCapacity:  getEnvInt64("DEDUP_BLOOM_CAPACITY", 10000000),
		BloomErrorRate: getEnvFloat("DEDUP_BLOOM_ERROR_RATE", 0.01),
	})
	if err != nil {
		log.Warn("dedup init failed, proceeding without dedup", "err", err)
	}

	// ── Ingester + Circuit Breaker ─────────────────────────────────
	baseIng, err := ingester.New(repo, sink, ingester.Config{
		Logger: log,
		Dedup:  dedupSeen,
	})
	must(log, "ingester", err)

	cb := resilience.NewCircuitBreaker(10, 30*time.Second)
	ing := ingester.NewResilient(baseIng, cb, log)

	// ── Parser ─────────────────────────────────────────────────────
	tracedQ := middleware.NewTracedSubscriber(parseQ, metricstypes.Stage("parse"), sink)
	w, err := parser.New(tracedQ, store, ing, sink, parserCfg)
	must(log, "parser", err)

	// ── Partition Maintenance ──────────────────────────────────────
	partitionAdmin := stores.Partitions
	if partitionAdmin != nil && parserCfg.PartitionMaintenanceInterval > 0 {
		go func() {
			ticker := time.NewTicker(parserCfg.PartitionMaintenanceInterval)
			defer ticker.Stop()

			// Run once immediately to catch up
			until := time.Now().AddDate(1, 0, 0)
			if err := partitionAdmin.EnsurePartitions(ctx, until); err != nil {
				log.Warn("initial partition maintenance failed", "err", err)
			} else {
				log.Info("initial partition maintenance completed", "until", until)
			}

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					until := time.Now().AddDate(1, 0, 0)
					if err := partitionAdmin.EnsurePartitions(ctx, until); err != nil {
						log.Warn("periodic partition maintenance failed", "err", err)
					} else {
						log.Info("partition maintenance completed", "until", until)
					}
				}
			}
		}()
	}

	// ── Run ────────────────────────────────────────────────────────
	log.Info("parser: starting")
	if err := w.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("parser: stopped", "err", err)
	}
	log.Info("parser: stopped cleanly")
}

// must exits the process if err is non-nil.
func must(log *slog.Logger, what string, err error) {
	if err != nil {
		log.Error(what, "err", err)
		os.Exit(1)
	}
}

// loadPostgresConfig reads POSTGRES_* environment variables into coreconfig.PostgresConfig.
func loadPostgresConfig() coreconfig.PostgresConfig {
	return coreconfig.PostgresConfig{
		DSN:             getEnv("POSTGRES_DSN", ""),
		MaxOpenConns:    getEnvInt("POSTGRES_MAX_OPEN_CONNS", 25),
		MaxIdleConns:    getEnvInt("POSTGRES_MAX_IDLE_CONNS", 5),
		ConnMaxLifetime: getEnvDur("POSTGRES_CONN_MAX_LIFETIME", 30*time.Minute),
		ConnMaxIdleTime: getEnvDur("POSTGRES_CONN_MAX_IDLE_TIME", 10*time.Minute),
	}
}

// ──────────────────────────────────────────────────────────────────
// Environment variable helpers
// ──────────────────────────────────────────────────────────────────

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getEnvInt64(key string, def int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}

func getEnvFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			return n
		}
	}
	return def
}

func getEnvBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

func getEnvDur(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
