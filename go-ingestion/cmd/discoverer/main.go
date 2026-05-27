package main

import (
	"context"
	"encoding/json"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	coreconfig "github.com/user-for-download/dota2-analysis/go-core/config"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/bootstrap"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/dedup/redisseen"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/metrics/otelmetrics"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/queue"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/queue/middleware"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/queue/redisstreams"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/proxy"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/proxy/httpdo"
	storageredis "github.com/user-for-download/dota2-analysis/go-ingestion/internal/storage/redis"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/storage/pgclient"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/worker/discovery"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/worker/discovery/matches"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/storage/matchstore"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/storage/herostatstore"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/storage/leaguestore"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/storage/proplayerstore"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/storage/teamstore"
)

func main() {
	log := bootstrap.NewLogger(slog.NewJSONHandler(os.Stdout, nil))

	fs := flag.NewFlagSet("discoverer", flag.ExitOnError)
	fileKey := fs.String("file", "", "run only this query key (filename without .sql); one-shot (matches only)")
	if err := fs.Parse(os.Args[1:]); err != nil {
		log.Error("parse flags", "err", err)
		os.Exit(2)
	}

	// ── Load configs from domain packages ──────────────────────────
	redisCfg := storageredis.LoadConfig()
	queueCfg := queue.LoadConfig()
	proxyCfg := proxy.LoadConfig()
	discCfg := discovery.LoadConfig()

	// ── Context with signal handling ──────────────────────────────
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// ── Telemetry ─────────────────────────────────────────────────
	shutdownTelemetry, err := bootstrap.InitTelemetry(ctx, "go-dota2-discoverer",
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

	// ── Metrics ───────────────────────────────────────────────────
	sink, err := otelmetrics.New()
	must(log, "metrics", err)

	// ── Redis ──────────────────────────────────────────────────────
	rdb, err := storageredis.New(redisCfg)
	must(log, "redis", err)
	defer rdb.Close()

	// ── Proxy pool ────────────────────────────────────────────────
	pool, err := bootstrap.ProxyPool(rdb.Master(), proxyCfg, log)
	must(log, "proxy pool", err)

	// ── Wait for proxies when direct access is disabled ──────────
	if !discCfg.AllowDirect {
		if err := bootstrap.WaitForProxies(ctx, pool, bootstrap.WaitConfig{
			MinSize:      discCfg.MinProxyPoolSize,
			Timeout:      discCfg.WaitTimeout,
			PollInterval: 2 * time.Second,
		}, log); err != nil {
			must(log, "wait for proxies", err)
		}
	}

	// ── Fetch queue (Publisher) ────────────────────────────────────
	rawQ, err := redisstreams.New(rdb.Master(), redisstreams.Config{
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
	fetchQ := middleware.NewTracedPublisher(rawQ)

	// ── Dedup ──────────────────────────────────────────────────────
	dedupSeen, err := redisseen.New(rdb.Master(), redisseen.Config{
		KeyPrefix:      getEnv("DEDUP_KEY_PREFIX", "dota2:seen"),
		TTL:            getEnvDur("DEDUP_TTL", 24*time.Hour),
		UseBloom:       getEnvBool("DEDUP_USE_BLOOM", false),
		BloomCapacity:  getEnvInt64("DEDUP_BLOOM_CAPACITY", 10000000),
		BloomErrorRate: getEnvFloat("DEDUP_BLOOM_ERROR_RATE", 0.01),
	})
	if err != nil {
		log.Warn("dedup init failed, proceeding without dedup", "err", err)
		dedupSeen = nil
	}

	// ── HTTP Doer ─────────────────────────────────────────────────
	doer, err := httpdo.New(httpdo.Config{
		Pool:        pool,
		Hold:        proxyCfg.Hold,
		Timeout:     discCfg.HTTPTimeout,
		MaxRetries:  discCfg.MaxRetries,
		Backoff:     discCfg.RetryBackoff,
		AllowDirect: discCfg.AllowDirect,
		Logger:      log,
	})
	must(log, "http doer", err)
	defer doer.Close()

	// ── Load queries ───────────────────────────────────────────────
	queries, err := discovery.LoadQueries(discCfg.QueriesDir)
	must(log, "load queries", err)
	log.Info("queries loaded", "dir", discCfg.QueriesDir, "count", len(queries))

	// ── Postgres (optional) ────────────────────────────────────────
	var pg *pgxpool.Pool
	if pgDSN := getEnv("POSTGRES_DSN", ""); pgDSN != "" {
		pg, err = bootstrap.WaitForPostgres(ctx, coreconfig.PostgresConfig{
			DSN:             pgDSN,
			MaxOpenConns:    getEnvInt("POSTGRES_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    getEnvInt("POSTGRES_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: getEnvDur("POSTGRES_CONN_MAX_LIFETIME", 30*time.Minute),
			ConnMaxIdleTime: getEnvDur("POSTGRES_CONN_MAX_IDLE_TIME", 10*time.Minute),
		}, bootstrap.WaitConfig{
			Timeout:      0,
			PollInterval: 30 * time.Second,
		}, log)
		if err != nil {
			log.Warn("Postgres not available; DB cycles disabled and DB deduplication disabled", "err", err)
			pg = nil
		}
	} else {
		log.Warn("Postgres DSN not provided; running without DB deduplication and DB cycles")
	}
	if pg != nil {
		defer pg.Close()
	}

	// ── Match reader (for DB-backed dedup) ─────────────────────────
	var matchReader = matchstore.MatchReader(nil)
	if pg != nil {
		matchReader = pgclient.NewStores(pg, log).Matches
	}

	// ── Matches cycle ──────────────────────────────────────────────
	mc, err := matches.New(fetchQ, doer, sink, matches.Config{
		ExplorerURL:   discCfg.UpstreamURL,
		Queries:       queries,
		DefaultKey:    discCfg.DefaultQueryKey,
		Interval:      discCfg.Interval,
		RunAtStart:    discCfg.RunAtStart,
		MaxRetries:    discCfg.MaxRetries,
		RetryBackoff:  discCfg.RetryBackoff,
		Logger:        log,
		Dedup:         dedupSeen,
		FileKey:       *fileKey,
		Reader:        matchReader,
	})
	must(log, "matches cycle", err)

	var cycles []discovery.Cycle
	cycles = append(cycles, mc)

	// ── League cycle (optional, requires PG) ───────────────────────
	if pg != nil && discCfg.LeagueQueriesDir != "" {
		lq, err := discovery.LoadQueries(discCfg.LeagueQueriesDir)
		if err != nil {
			log.Warn("league queries load failed, skipping", "err", err)
		} else if len(lq) > 0 {
			sql := pickQuery(lq, "default")
			repo := leaguestore.NewPG(pg)
			lc := discovery.NewExplorerCycle(
				"leagues", doer, discCfg.UpstreamURL, sql, discCfg.LeagueInterval, log,
				func(rows []json.RawMessage) ([]leaguestore.League, error) {
					out := make([]leaguestore.League, 0)
					for _, r := range rows {
						var row leagueRow
						if err := json.Unmarshal(r, &row); err != nil {
							continue
						}
						if row.LeagueID == 0 {
							continue
						}
						out = append(out, leaguestore.League{
							LeagueID: row.LeagueID,
							Name:     derefStr(row.Name),
							Tier:     derefStr(row.Tier),
							Ticket:   derefStr(row.Ticket),
							Banner:   derefStr(row.Banner),
						})
					}
					return out, nil
				},
				repo.Upsert,
			)
			cycles = append(cycles, lc)
		}
	}

	// ── Team cycle (optional, requires PG) ─────────────────────────
	if pg != nil && discCfg.TeamQueriesDir != "" {
		tq, err := discovery.LoadQueries(discCfg.TeamQueriesDir)
		if err != nil {
			log.Warn("team queries load failed, skipping", "err", err)
		} else if len(tq) > 0 {
			sql := pickQuery(tq, "default")
			repo := teamstore.NewPG(pg)
			tc := discovery.NewExplorerCycle(
				"teams", doer, discCfg.UpstreamURL, sql, discCfg.TeamInterval, log,
				func(rows []json.RawMessage) ([]teamstore.Team, error) {
					out := make([]teamstore.Team, 0)
					for _, r := range rows {
						var row teamRow
						if err := json.Unmarshal(r, &row); err != nil {
							continue
						}
						if row.TeamID == 0 {
							continue
						}
						out = append(out, teamstore.Team{
							TeamID:        row.TeamID,
							Name:         derefStr(row.Name),
							Tag:          derefStr(row.Tag),
							LogoURL:      derefStr(row.LogoURL),
							Rating:       row.Rating,
							Wins:         row.Wins,
							Losses:       row.Losses,
							LastMatchTime: row.LastMatchTime,
							Delta:        row.Delta,
							MatchID:      row.MatchID,
						})
					}
					return out, nil
				},
				repo.Upsert,
			)
			cycles = append(cycles, tc)
		}
	}

	// ── ProPlayer cycle (optional, requires PG) ────────────────────
	if pg != nil && discCfg.ProPlayerURL != "" {
		repo := proplayerstore.NewPG(pg)
		pc := discovery.NewHTTPCycle(
			"proplayers", doer, discCfg.ProPlayerURL, discCfg.ProPlayerInterval, log,
			func(body []byte) ([]proplayerstore.ProPlayer, error) {
				var players []proplayerstore.ProPlayer
				if err := json.Unmarshal(body, &players); err != nil {
					return nil, err
				}
				return players, nil
			},
			repo.Upsert,
		)
		cycles = append(cycles, pc)
	}

	// ── HeroStats cycle (optional, requires PG) ────────────────────
	if pg != nil && discCfg.HeroStatsURL != "" {
		repo := herostatstore.NewPG(pg)
		hc := discovery.NewHTTPCycle(
			"herostats", doer, discCfg.HeroStatsURL, discCfg.HeroStatsInterval, log,
			func(body []byte) ([]herostatstore.HeroStat, error) {
				var stats []herostatstore.HeroStat
				if err := json.Unmarshal(body, &stats); err != nil {
					return nil, err
				}
				return stats, nil
			},
			repo.Upsert,
		)
		cycles = append(cycles, hc)
	}

	// ── One-shot mode ──────────────────────────────────────────────
	if *fileKey != "" {
		if err := mc.RunOnce(ctx); err != nil {
			log.Error("one-shot failed", "err", err)
			os.Exit(1)
		}
		log.Info("one-shot done")
		return
	}

	// ── Run ────────────────────────────────────────────────────────
	scheduler := discovery.NewScheduler(cycles, log)
	scheduler.Run(ctx)
}

// ── Row types for inline JSON decoding ──────────────────────────────

type leagueRow struct {
	LeagueID int64   `json:"leagueid"`
	Name     *string `json:"name"`
	Tier     *string `json:"tier"`
	Ticket   *string `json:"ticket"`
	Banner   *string `json:"banner"`
}

type teamRow struct {
	TeamID        int64    `json:"team_id"`
	Name         *string  `json:"name"`
	Tag          *string  `json:"tag"`
	LogoURL      *string  `json:"logo_url"`
	Rating       *float64 `json:"rating"`
	Wins         *int     `json:"wins"`
	Losses       *int     `json:"losses"`
	LastMatchTime *int64  `json:"last_match_time"`
	Delta        *float64 `json:"delta"`
	MatchID      *int64   `json:"match_id"`
}

func pickQuery(queries map[string]string, fallback string) string {
	if sql, ok := queries[fallback]; ok {
		return sql
	}
	for _, v := range queries {
		return v
	}
	return ""
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ── Helpers ────────────────────────────────────────────────────────

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
