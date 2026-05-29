package matchpg

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/user-for-download/dota2-analysis/go-core/domain"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/metrics"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/storage/matchstore"
)

type Store struct {
	db  *pgxpool.Pool
	m   metrics.Sink // optional; nil at bootstrap, set by caller via SetMetrics
	log *slog.Logger
}

func NewStore(db *pgxpool.Pool, log *slog.Logger) *Store {
	if log == nil {
		log = slog.Default()
	}
	return &Store{db: db, log: log.With("component", "matchpg")}
}

// SetMetrics attaches an optional metrics sink for draft-schema canary alerts.
// Must be called before the first IngestMatch call if metrics are desired.
func (s *Store) SetMetrics(m metrics.Sink) { s.m = m }

var _ matchstore.MatchWriter = (*Store)(nil)
var _ matchstore.MatchReader = (*Store)(nil)

func (s *Store) IngestMatch(ctx context.Context, m domain.Match) error {
	if m.MatchID == 0 || m.StartTime == 0 {
		return fmt.Errorf("match: id and start_time required")
	}
	s.log.Debug("matchpg: starting ingestion transaction", "match_id", int64(m.MatchID), "parsed", m.IsParsed)

	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		heroIDs := collectHeroIDs(m)
		if len(heroIDs) > 0 {
			if err := s.ensureHeroStubs(ctx, tx, heroIDs); err != nil {
				s.log.Warn("ensure_hero_stubs failed, triggers will handle missing heroes",
					"match_id", int64(m.MatchID), "err", err)
			}
		}

		s.log.Debug("matchpg: upserting match root", "match_id", int64(m.MatchID))
		_, err := s.upsertMatchRoot(ctx, tx, m)
		if err != nil {
			s.log.Error("matchpg: failed to upsert match root", "match_id", int64(m.MatchID), "err", err)
			return err
		}

		if err := s.upsertTeamMatches(ctx, tx, m); err != nil {
			return err
		}

		if err := s.upsertPlayers(ctx, tx, m); err != nil {
			return err
		}

		if m.IsParsed {
			if err := s.upsertPlayerDetails(ctx, tx, m); err != nil {
				return err
			}
			if err := s.upsertPicksBans(ctx, tx, m); err != nil {
				return err
			}
			if err := s.checkDraftSchema(ctx, m); err != nil {
				return err // rollback → match sent to DLQ
			}
			if err := s.upsertDraftTimings(ctx, tx, m); err != nil {
				return err
			}
			if err := s.upsertAdvantages(ctx, tx, m); err != nil {
				return err
			}
			if err := s.replaceObjectives(ctx, tx, m); err != nil {
				return err
			}
			if err := s.replaceChat(ctx, tx, m); err != nil {
				return err
			}
			if err := s.replaceTeamfights(ctx, tx, m); err != nil {
				return err
			}
			if err := s.upsertCosmetics(ctx, tx, m); err != nil {
				return err
			}
			if err := s.upsertTimeseries(ctx, tx, m); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		s.log.Error("matchpg: ingestion transaction failed", "match_id", int64(m.MatchID), "err", err)
		return err
	}
	s.log.Info("match ingested", "match_id", int64(m.MatchID), "parsed", m.IsParsed)
	return nil
}

func (s *Store) UnknownIDs(ctx context.Context, candidates []matchstore.MatchRef) ([]int64, error) {
	if len(candidates) == 0 {
		return nil, nil
	}
	// Flatten two parallel slices: match_ids[] and start_times[].
	matchIDs := make([]int64, len(candidates))
	startTimes := make([]int64, len(candidates))
	for i, c := range candidates {
		matchIDs[i] = c.MatchID
		startTimes[i] = c.StartTime
	}
	// Filter by both match_id AND start_time so PostgreSQL can prune to
	// the relevant quarterly partition(s) instead of scanning all 20+.
	rows, err := s.db.Query(ctx, `
		SELECT c.match_id
		FROM unnest($1::bigint[], $2::bigint[]) AS c(match_id, start_time)
		LEFT JOIN matches m ON m.match_id = c.match_id AND m.start_time = c.start_time
		WHERE m.match_id IS NULL OR m.is_parsed = FALSE
	`, matchIDs, startTimes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *Store) Counts(ctx context.Context) (matchstore.Counts, error) {
	var c matchstore.Counts
	err := s.db.QueryRow(ctx, `
		SELECT
			(SELECT COALESCE(sum(reltuples::bigint), 0)
			 FROM pg_class c
			 JOIN pg_inherits i ON i.inhrelid = c.oid
			 JOIN pg_class p ON p.oid = i.inhparent
			 JOIN pg_namespace n ON n.oid = p.relnamespace
			 WHERE n.nspname = 'public'
			   AND p.relname = 'matches'),
			(SELECT COALESCE(sum(reltuples::bigint), 0)
			 FROM pg_class c
			 JOIN pg_inherits i ON i.inhrelid = c.oid
			 JOIN pg_class p ON p.oid = i.inhparent
			 JOIN pg_namespace n ON n.oid = p.relnamespace
			 WHERE n.nspname = 'public'
			   AND p.relname = 'player_matches')
	`).Scan(&c.Matches, &c.Players)
	return c, err
}

func (s *Store) IsIngested(ctx context.Context, matchID int64, startTime int64) (bool, error) {
	var exists bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM matches WHERE match_id = $1 AND start_time = $2)`, matchID, startTime).Scan(&exists)
	return exists, err
}

