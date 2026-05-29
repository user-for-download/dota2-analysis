package main

import (
	"context"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/bootstrap"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/metrics/otelmetrics"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/proxy"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/proxy/loader"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/proxy/redisproxy"
	storageredis "github.com/user-for-download/dota2-analysis/go-ingestion/internal/storage/redis"
)

func main() {
	log := bootstrap.NewLoggerFromEnv()

	// Load domain-specific configs directly — no god-config needed.
	redisCfg := storageredis.LoadConfig()
	proxyCfg := proxy.LoadConfig()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdownTelemetry, err := bootstrap.InitTelemetry(ctx, "go-dota2-proxyloader",
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

	redisClient, err := storageredis.New(redisCfg)
	must(log, "redis", err)
	defer redisClient.Close()
	rdb := redisClient.Master()

	// Metrics sink for proxy operations.
	m, err := otelmetrics.New()
	must(log, "otelmetrics", err)
	_ = m // available for future use

	// Create the proxy pool directly from the local proxy config, bypassing the
	// global bootstrap to break the dependency on config.ProxyConfig.
	pool, err := redisproxy.New(rdb, redisproxy.Config{
		KeyPrefix: proxyCfg.KeyPrefix,
		RateLimit: proxy.RateLimit{
			RatePerSec: proxyCfg.RateLimitPerSec,
			Burst:      proxyCfg.RateLimitBurst,
			Window:     proxyCfg.RateLimitWindow,
		},
		Ranking: proxy.Ranking{
			InitialWeight:  proxyCfg.RankingInitial,
			SuccessBoost:   proxyCfg.RankingSuccess,
			FailurePenalty: proxyCfg.RankingFailure,
		},
		MaxFailures: proxyCfg.MaxFailures,
		Logger:      log,
	})
	must(log, "proxy pool", err)

	seedPath := proxyCfg.SeedFile
	if seedPath == "" {
		seedPath = "proxy.txt"
	}
	canary := proxyCfg.CanaryURL
	if canary == "" {
		canary = "https://api.ipify.org"
	}

	seed := loader.FileSource{Path: seedPath}
	var remote loader.Source
	if proxyCfg.RemoteURL != "" {
		remote = loader.HTTPSource{
			URL:    proxyCfg.RemoteURL,
			Client: &http.Client{Timeout: 15 * time.Second},
		}
	}

	minPoolSize := int64(proxyCfg.MinPoolSize)
	if minPoolSize <= 0 {
		minPoolSize = 20
	}

	// top-up interval: check pool size, reload only when below threshold
	topUpInterval := proxyCfg.RefreshInterval

	// force-refresh interval: unconditional full reload to evict degraded proxies.
	// Reads PROXY_FORCE_REFRESH_INTERVAL; defaults to 1h. Set to 0 to disable.
	forceInterval := getEnvDuration("PROXY_FORCE_REFRESH_INTERVAL", time.Hour)

	ld, err := loader.New(pool, loader.Config{
		Seed:   seed,
		Remote: remote,
		Validate: loader.Validator{
			CanaryURL: canary,
			Timeout:   proxyCfg.ValidateTimeout,
		},
		Parallel:   proxyCfg.ValidateParallel,
		ChunkSize:  proxyCfg.ValidateChunkSize,
		MinPublish: proxyCfg.ValidateMinPublish,
		Logger:     log,
	})
	must(log, "loader", err)

	// initial load
	if err := ld.Run(ctx); err != nil {
		if topUpInterval <= 0 && forceInterval <= 0 {
			log.Error("initial proxy load failed (one-shot)", "err", err)
			os.Exit(1)
		}
		log.Error("initial proxy load failed; will retry on schedule", "err", err)
	} else {
		if topUpInterval <= 0 && forceInterval <= 0 {
			log.Info("one-shot mode; exiting")
			return
		}
	}

	// nextTopUpTick adds 0–10% positive jitter to the top-up interval to
	// desynchronize proxyloader instances across pods. Uses a timer (not a
	// ticker) so the jitter is re-evaluated on every cycle — otherwise a
	// ticker evaluates the duration once and pods stay locked in phase.
	nextTopUpTick := func() time.Duration {
		if topUpInterval <= 0 || topUpInterval < time.Second {
			return topUpInterval
		}
		jitter := time.Duration(rand.Int63n(int64(topUpInterval / 10)))
		return topUpInterval + jitter
	}

	// build timers; a nil channel blocks forever, so disabled intervals are
	// represented as a nil *time.Timer with a nil C channel.
	var topUpC <-chan time.Time
	var topUpTimer *time.Timer
	if topUpInterval > 0 {
		topUpTimer = time.NewTimer(nextTopUpTick())
		defer topUpTimer.Stop()
		topUpC = topUpTimer.C
	}

	var forceC <-chan time.Time
	if forceInterval > 0 {
		t := time.NewTicker(forceInterval)
		defer t.Stop()
		forceC = t.C
		log.Info("force-refresh enabled", "interval", forceInterval)
	}

	for {
		select {
		case <-ctx.Done():
			log.Info("stopped cleanly")
			return

		case <-forceC:
			// Unconditional reload: re-validates every proxy in the seed/remote
			// list regardless of current pool size. This evicts proxies that are
			// technically still in the ZSET but have degraded to near-eviction
			// failure counts before the max-failures threshold removes them.
			log.Info("force-refresh: reloading and re-validating full proxy list")
			if err := ld.Run(ctx); err != nil {
				log.Warn("force-refresh failed; keeping existing pool", "err", err)
			}

		case <-topUpC:
			topUpTimer.Reset(nextTopUpTick())
			// Conditional reload: only runs when the pool has dropped below the
			// minimum threshold. Handles sudden eviction bursts between force ticks.
			size, err := pool.Size(ctx)
			if err != nil {
				log.Warn("pool size check failed", "err", err)
				continue
			}
			if size >= int(minPoolSize) {
				log.Debug("top-up: pool healthy; skipping", "size", size, "min", minPoolSize)
				continue
			}
			log.Info("top-up: pool below threshold; refreshing", "size", size, "min", minPoolSize)
			if err := ld.Run(ctx); err != nil {
				log.Warn("top-up refresh failed; keeping existing pool", "err", err)
			}
		}
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
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

// getEnvDuration reads a duration from an environment variable.
// Returns the provided default if the variable is unset or unparseable.
func getEnvDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

func must(log *slog.Logger, what string, err error) {
	if err != nil {
		log.Error(what, "err", err)
		os.Exit(1)
	}
}