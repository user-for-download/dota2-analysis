package bootstrap

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/config"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/dedup"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/metrics"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/queue"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/storage/herostatstore"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/storage/leaguestore"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/storage/matchstore"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/storage/pgclient"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/storage/proplayerstore"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/storage/teamstore"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/worker/discovery"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/worker/discovery/matches"
)

type DiscovererDeps struct {
	Pool    *pgxpool.Pool
	Cycles  []discovery.Cycle
	Matches discovery.Cycle
}

func BuildDiscoverer(
	ctx context.Context,
	cfg *config.Config,
	log *slog.Logger,
	fetchQ queue.Publisher,
	doer discovery.HTTPDoer,
	dedupSeen dedup.Seen,
	metricS metrics.Sink,
	queries map[string]string,
	fileKey string,
) (*DiscovererDeps, error) {
	deps := &DiscovererDeps{}

	var pg *pgxpool.Pool
	if cfg.Postgres.DSN != "" {
		var err error
		pg, err = WaitForPostgres(ctx, cfg.Postgres, WaitConfig{
			Timeout:      0,
			PollInterval: 30 * time.Second,
		}, log)
		if err != nil {
			log.Warn("Postgres not available; DB cycles disabled and DB deduplication disabled", "err", err)
			pg = nil
		}
		deps.Pool = pg
	} else {
		log.Warn("Postgres DSN not provided; running without DB deduplication and DB cycles")
	}

	var matchReader matchstore.MatchReader
	if pg != nil {
		matchReader = pgclient.NewStores(pg, log).Matches
	}

	mc, err := matches.New(fetchQ, doer, metricS, matches.Config{
		ExplorerURL: cfg.Discovery.UpstreamURL,
		Queries:     queries,
		DefaultKey:  cfg.Discovery.DefaultQueryKey,
		Interval:    cfg.Discovery.Interval,
		RunAtStart:  cfg.Discovery.RunAtStart,
		Logger:      log,
		Dedup:       dedupSeen,
		FileKey:     fileKey,
		Reader:      matchReader,
	})
	if err != nil {
		return nil, err
	}
	deps.Matches = mc
	deps.Cycles = append(deps.Cycles, mc)

	if pg != nil && cfg.Discovery.LeagueQueriesDir != "" {
		lq, err := discovery.LoadQueries(cfg.Discovery.LeagueQueriesDir)
		if err != nil {
			log.Warn("league queries load failed, skipping", "err", err)
		} else if len(lq) > 0 {
			sql := pickQuery(lq, "default")
			lc := buildLeagueCycle(doer, pg, cfg.Discovery.UpstreamURL, sql, cfg.Discovery.LeagueInterval, log)
			deps.Cycles = append(deps.Cycles, lc)
		}
	}

	if pg != nil && cfg.Discovery.TeamQueriesDir != "" {
		tq, err := discovery.LoadQueries(cfg.Discovery.TeamQueriesDir)
		if err != nil {
			log.Warn("team queries load failed, skipping", "err", err)
		} else if len(tq) > 0 {
			sql := pickQuery(tq, "default")
			tc := buildTeamCycle(doer, pg, cfg.Discovery.UpstreamURL, sql, cfg.Discovery.TeamInterval, log)
			deps.Cycles = append(deps.Cycles, tc)
		}
	}

	if pg != nil && cfg.Discovery.ProPlayerURL != "" {
		pc := buildProPlayerCycle(doer, pg, cfg.Discovery.ProPlayerURL, cfg.Discovery.ProPlayerInterval, log)
		deps.Cycles = append(deps.Cycles, pc)
	}

	if pg != nil && cfg.Discovery.HeroStatsURL != "" {
		hc := buildHeroStatsCycle(doer, pg, cfg.Discovery.HeroStatsURL, cfg.Discovery.HeroStatsInterval, log)
		deps.Cycles = append(deps.Cycles, hc)
	}

	return deps, nil
}

type leagueRow struct {
	LeagueID int64   `json:"leagueid"`
	Name     *string `json:"name"`
	Tier     *string `json:"tier"`
	Ticket   *string `json:"ticket"`
	Banner   *string `json:"banner"`
}

func buildLeagueCycle(doer discovery.HTTPDoer, pg *pgxpool.Pool, explorerURL, sql string, interval time.Duration, log *slog.Logger) discovery.Cycle {
	repo := leaguestore.NewPG(pg)
	return discovery.NewExplorerCycle[leaguestore.League](
		"leagues", doer, explorerURL, sql, interval, log,
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
}

type teamRow struct {
	TeamID        int64   `json:"team_id"`
	Name         *string `json:"name"`
	Tag          *string `json:"tag"`
	LogoURL      *string `json:"logo_url"`
	Rating       *float64 `json:"rating"`
	Wins         *int    `json:"wins"`
	Losses       *int    `json:"losses"`
	LastMatchTime *int64  `json:"last_match_time"`
	Delta        *float64 `json:"delta"`
	MatchID      *int64  `json:"match_id"`
}

func buildTeamCycle(doer discovery.HTTPDoer, pg *pgxpool.Pool, explorerURL, sql string, interval time.Duration, log *slog.Logger) discovery.Cycle {
	repo := teamstore.NewPG(pg)
	return discovery.NewExplorerCycle[teamstore.Team](
		"teams", doer, explorerURL, sql, interval, log,
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
}

func buildProPlayerCycle(doer discovery.HTTPDoer, pg *pgxpool.Pool, url string, interval time.Duration, log *slog.Logger) discovery.Cycle {
	repo := proplayerstore.NewPG(pg)
	return discovery.NewHTTPCycle[proplayerstore.ProPlayer](
		"proplayers", doer, url, interval, log,
		func(body []byte) ([]proplayerstore.ProPlayer, error) {
			var players []proplayerstore.ProPlayer
			if err := json.Unmarshal(body, &players); err != nil {
				return nil, err
			}
			return players, nil
		},
		repo.Upsert,
	)
}

func buildHeroStatsCycle(doer discovery.HTTPDoer, pg *pgxpool.Pool, url string, interval time.Duration, log *slog.Logger) discovery.Cycle {
	repo := herostatstore.NewPG(pg)
	return discovery.NewHTTPCycle[herostatstore.HeroStat](
		"herostats", doer, url, interval, log,
		func(body []byte) ([]herostatstore.HeroStat, error) {
			var stats []herostatstore.HeroStat
			if err := json.Unmarshal(body, &stats); err != nil {
				return nil, err
			}
			return stats, nil
		},
		repo.Upsert,
	)
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