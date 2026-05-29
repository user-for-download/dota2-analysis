package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	coreconfig "github.com/user-for-download/dota2-analysis/go-core/config"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/bootstrap"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/enrich"
	enrichgate "github.com/user-for-download/dota2-analysis/go-ingestion/internal/enrich/gate"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/enrich/httpclient"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/enrich/sources/dotaconstants"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/proxy"
	storageredis "github.com/user-for-download/dota2-analysis/go-ingestion/internal/storage/redis"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/storage/refdatastore"
)

// enrichConfig holds the enricher-specific configuration loaded from environment variables.
type enrichConfig struct {
	DotaConstantsBaseURL string
	HTTPTimeout          time.Duration
	AllowDirect          bool
	MaxProxyRetries      int
	ProxyBackoff         time.Duration
	ForceBootstrap       bool
	BootstrapPrefix      string
	Interval             time.Duration
	RunAtStart           bool
	LocalBootstrapDir    string
	WaitTimeout          time.Duration
}

func loadEnrichConfig() enrichConfig {
	return enrichConfig{
		DotaConstantsBaseURL: getEnv("ENRICH_DOTACONSTANTS_BASE_URL", "https://raw.githubusercontent.com/odota/dotaconstants/master/build"),
		HTTPTimeout:          getEnvDur("ENRICH_HTTP_TIMEOUT", 30*time.Second),
		AllowDirect:          getEnvBool("ENRICH_ALLOW_DIRECT", false),
		MaxProxyRetries:      getEnvInt("ENRICH_MAX_PROXY_RETRIES", 5),
		ProxyBackoff:         getEnvDur("ENRICH_PROXY_BACKOFF", 500*time.Millisecond),
		ForceBootstrap:       getEnvBool("ENRICH_FORCE_BOOTSTRAP", false),
		BootstrapPrefix:      getEnv("ENRICH_BOOTSTRAP_PREFIX", "dota2:enrich"),
		Interval:             getEnvDur("ENRICH_INTERVAL", 24*time.Hour),
		RunAtStart:           getEnvBool("ENRICH_RUN_AT_START", true),
		LocalBootstrapDir:    getEnv("ENRICH_LOCAL_DIR", ""),
		WaitTimeout:          getEnvDur("ENRICH_WAIT_TIMEOUT", 5*time.Minute),
	}
}

func main() {
	log := bootstrap.NewLoggerFromEnv()

	// ── Load configs from domain packages ──────────────────────────
	redisCfg := storageredis.LoadConfig()
	proxyCfg := proxy.LoadConfig()
	enrichCfg := loadEnrichConfig()

	// ── Context with signal handling ──────────────────────────────
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// ── Telemetry ─────────────────────────────────────────────────
	shutdownTelemetry, err := bootstrap.InitTelemetry(ctx, "go-dota2-enricher",
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

	// ── Redis client ──────────────────────────────────────────────
	rdb, err := storageredis.New(redisCfg)
	must(log, "redis", err)
	defer rdb.Close()

	// ── Proxy pool ────────────────────────────────────────────────
	pool, err := bootstrap.ProxyPool(ctx, rdb.Master(), proxyCfg, log)
	must(log, "proxy pool", err)

	// ── Wait for proxies when direct access is disabled ───────────
	if !enrichCfg.AllowDirect {
		if err := bootstrap.WaitForProxies(ctx, pool, bootstrap.WaitConfig{
			MinSize:      proxyCfg.MinPoolSize,
			Timeout:      enrichCfg.WaitTimeout,
			PollInterval: 2 * time.Second,
		}, log); err != nil {
			must(log, "wait for proxies", err)
		}
	}

	// ── Postgres ──────────────────────────────────────────────────
	pgCfg := loadPostgresConfig()
	db, err := bootstrap.WaitForPostgres(ctx, pgCfg, bootstrap.WaitConfig{
		Timeout:      0,
		PollInterval: 30 * time.Second,
	}, log)
	must(log, "postgres", err)
	defer db.Close()

	// ── Reference data writer ─────────────────────────────────────
	repo := bootstrap.ReferenceWriter(db, log)

	// ── Remote HTTP client (proxy-aware) ──────────────────────────
	remoteHTTP, err := httpclient.NewProxied(httpclient.ProxiedConfig{
		Pool:        pool,
		Hold:        proxyCfg.Hold,
		Timeout:     enrichCfg.HTTPTimeout,
		Fallback:    &http.Client{Timeout: enrichCfg.HTTPTimeout},
		AllowDirect: enrichCfg.AllowDirect,
		MaxRetries:  enrichCfg.MaxProxyRetries,
		Backoff:     enrichCfg.ProxyBackoff,
		Logger:      log,
	})
	must(log, "remote http client", err)
	defer remoteHTTP.Close()

	// ── Sources ───────────────────────────────────────────────────
	remoteSrcs := buildSources(enrichCfg.DotaConstantsBaseURL, remoteHTTP, repo)

	// ── Gate ──────────────────────────────────────────────────────
	var runGate enrichgate.RunGate
	if enrichCfg.ForceBootstrap {
		log.Warn("enricher: ForceBootstrap enabled — gate bypassed, all sources will run unconditionally")
		runGate = enrichgate.Always{}
	} else {
		runGate = enrichgate.New(enrichgate.Config{
			Prefix: enrichCfg.BootstrapPrefix,
			MinAge: enrichCfg.Interval,
			TTL:    enrichCfg.Interval,
			Mode:   enrichgate.Interval,
			Client: rdb.Master(),
		})
	}

	// ── Main runner ───────────────────────────────────────────────
	mainRunner, err := enrich.NewRunner(enrich.RunnerOptions{
		Sources: remoteSrcs,
		HTTP:    remoteHTTP,
		Gate:    runGate,
		Logger:  log,
	})
	must(log, "main runner", err)

	// ── Local runner (file-based bootstrap) ───────────────────────
	var localRunner enrich.RunnerIface
	if enrichCfg.LocalBootstrapDir != "" {
		localBase := "file://" + enrichCfg.LocalBootstrapDir
		localHTTP := httpclient.NewFileClient()
		localSrcs := buildSources(localBase, localHTTP, repo)

		localRunner, err = enrich.NewRunner(enrich.RunnerOptions{
			Sources: localSrcs,
			HTTP:    localHTTP,
			Gate:    enrichgate.Always{},
			Logger:  log,
		})
		if err != nil {
			log.Warn("enricher: local runner init failed", "err", err)
			localRunner = nil
		}
	}

	// ── Run local bootstrap if applicable ─────────────────────────
	if localRunner != nil {
		log.Info("enricher: running local bootstrap from " + enrichCfg.LocalBootstrapDir)
		if err := localRunner.Run(ctx); err != nil {
			log.Warn("enricher: local bootstrap failed, falling back to remote", "err", err)
		} else {
			log.Info("enricher: local bootstrap completed successfully")
		}
	}

	// ── Run loop ──────────────────────────────────────────────────
	run := func() {
		log.Info("enricher: running enrichment cycle")
		if err := mainRunner.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Error("enricher: run failed", "err", err)
		}
	}

	if enrichCfg.RunAtStart {
		run()
	}

	if enrichCfg.Interval <= 0 {
		log.Info("enricher: interval <= 0, exiting")
		return
	}

	ticker := time.NewTicker(enrichCfg.Interval)
	defer ticker.Stop()
	log.Info("enricher: listening on interval schedule", "interval", enrichCfg.Interval)

	for {
		select {
		case <-ctx.Done():
			log.Info("enricher: stopped cleanly")
			return
		case <-ticker.C:
			run()
		}
	}
}

// buildSources constructs the full set of dotaconstants enrichment sources.
func buildSources(baseURL string, http enrich.HTTPClient, writer refdatastore.RefDataWriter) []enrich.RunSource {
	return []enrich.RunSource{
		dotaconstants.NewHeroesSource(baseURL, writer, http),
		dotaconstants.NewAbilitiesSource(baseURL, writer, http),
		dotaconstants.NewAbilityIDsSource(baseURL, writer, http),
		dotaconstants.NewHeroAbilitiesSource(baseURL, writer, http),
		dotaconstants.NewGameModesSource(baseURL, writer, http),
		dotaconstants.NewLobbyTypesSource(baseURL, writer, http),
		dotaconstants.NewRegionsSource(baseURL, writer, http),
		dotaconstants.NewItemsSource(baseURL, writer, http),
		dotaconstants.NewItemIDsSource(baseURL, writer, http),
		dotaconstants.NewPatchesSource(baseURL, writer, http),
	}
}

// ──────────────────────────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────────────────────────

func loadPostgresConfig() coreconfig.PostgresConfig {
	return coreconfig.PostgresConfig{
		DSN:             getEnv("POSTGRES_DSN", ""),
		MaxOpenConns:    getEnvInt("POSTGRES_MAX_OPEN_CONNS", 25),
		MaxIdleConns:    getEnvInt("POSTGRES_MAX_IDLE_CONNS", 5),
		ConnMaxLifetime: getEnvDur("POSTGRES_CONN_MAX_LIFETIME", 30*time.Minute),
		ConnMaxIdleTime: getEnvDur("POSTGRES_CONN_MAX_IDLE_TIME", 10*time.Minute),
	}
}

func must(log *slog.Logger, what string, err error) {
	if err != nil {
		log.Error(what, "err", err)
		os.Exit(1)
	}
}

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

func getEnvFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
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
