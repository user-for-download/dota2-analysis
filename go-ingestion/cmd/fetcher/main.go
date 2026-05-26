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

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/bootstrap"
	metricstypes "github.com/user-for-download/dota2-analysis/go-ingestion/internal/metrics"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/metrics/otelmetrics"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/payload"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/payload/redisstore"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/proxy"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/proxy/httpdo"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/queue"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/queue/middleware"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/queue/redisstreams"
	storageredis "github.com/user-for-download/dota2-analysis/go-ingestion/internal/storage/redis"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/worker/fetcher"
)

func main() {
	log := bootstrap.NewLogger(slog.NewJSONHandler(os.Stdout, nil))

	// ── Load individual configs instead of monolithic config.Load() ──
	redisCfg := storageredis.LoadConfig()
	queueCfg := queue.LoadConfig()
	payloadCfg := payload.LoadConfig()
	fetcherCfg := fetcher.LoadConfig()
	proxyCfg := proxy.LoadConfig()

	// ── Context with OS signal handling ──────────────────────────────
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// ── OpenTelemetry (env vars read directly until telemetry has its own package) ──
	otelEndpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	otelSampleRate := 1.0
	if s := os.Getenv("OTEL_SAMPLE_RATE"); s != "" {
		if v, err := strconv.ParseFloat(s, 64); err == nil {
			otelSampleRate = v
		}
	}

	shutdownTelemetry, err := bootstrap.InitTelemetry(ctx, "go-dota2-fetcher", otelEndpoint, otelSampleRate)
	if err != nil {
		log.Error("init telemetry", "err", err)
	} else if shutdownTelemetry != nil {
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = shutdownTelemetry(shutdownCtx)
		}()
	}

	// ── Metrics ──────────────────────────────────────────────────────
	metrics, err := otelmetrics.New()
	must(log, "otel metrics", err)

	// ── Redis client ─────────────────────────────────────────────────
	rdb, err := storageredis.New(redisCfg)
	must(log, "redis", err)
	defer rdb.Close()

	// ── Proxy pool ───────────────────────────────────────────────────
	pool, err := bootstrap.ProxyPool(rdb.Master(), proxyCfg, log)
	must(log, "proxy pool", err)

	// ── Wait for proxies when direct access is disabled ──────────────
	if !fetcherCfg.AllowDirect {
		if err := bootstrap.WaitForProxies(ctx, pool, bootstrap.WaitConfig{
			MinSize:      proxyCfg.MinPoolSize,
			Timeout:      fetcherCfg.WaitTimeout,
			PollInterval: 2 * time.Second,
		}, log); err != nil {
			must(log, "wait for proxies", err)
		}
	}

	// ── Payload store ────────────────────────────────────────────────
	store, err := redisstore.New(rdb.Master(), redisstore.Config{
		KeyPrefix:  payloadCfg.KeyPrefix,
		DefaultTTL: payloadCfg.DefaultTTL,
	})
	must(log, "payload store", err)

	// ── Fetch queue (subscriber: reads fetch stream) ─────────────────
	sub, err := redisstreams.New(rdb.Master(), redisstreams.Config{
		Stream:      queueCfg.FetchStream,
		DLQStream:   queueCfg.FetchDLQStream,
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
	must(log, "fetch queue", err)

	// ── Parse queue (publisher: writes to parse stream) ──────────────
	pub, err := redisstreams.New(rdb.Master(), redisstreams.Config{
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

	// ── HTTP Doer (proxy-aware HTTP client) ──────────────────────────
	doer, err := httpdo.New(httpdo.Config{
		Pool:        pool,
		Hold:        proxyCfg.Hold,
		Timeout:     fetcherCfg.HTTPTimeout,
		MaxRetries:  fetcherCfg.MaxProxyRetries,
		Backoff:     fetcherCfg.ProxyBackoff,
		AllowDirect: fetcherCfg.AllowDirect,
		Logger:      log,
	})
	must(log, "http doer", err)
	defer doer.Close()

	// ── Apply Telemetry Decorators ───────────────────────────────────
	// Wrap the raw queues so OpenTelemetry context headers are automatically
	// injected into published messages and extracted from received messages.
	tracedSub := middleware.NewTracedSubscriber(sub, metricstypes.Stage("fetch"), metrics)
	tracedPub := middleware.NewTracedPublisher(pub)

	// ── Fetcher worker ───────────────────────────────────────────────
	// Pass the decorated queues to the worker so OTel traces link Fetch→Parse.
	w, err := fetcher.New(tracedSub, tracedPub, doer, store, metrics, fetcher.Config{
		UpstreamURL: fetcherCfg.UpstreamURL,
		Batch:       fetcherCfg.Batch,
		Block:       fetcherCfg.Block,
		HTTPTimeout: fetcherCfg.HTTPTimeout,
		PayloadTTL:  fetcherCfg.PayloadTTL,
		Logger:      log,
	})
	must(log, "build fetcher", err)

	// ── Run ──────────────────────────────────────────────────────────
	log.Info("fetcher: starting")
	if err := w.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("fetcher: stopped", "err", err)
	}
	log.Info("fetcher: stopped cleanly")
}

func must(log *slog.Logger, what string, err error) {
	if err != nil {
		log.Error(what, "err", err)
		os.Exit(1)
	}
}
