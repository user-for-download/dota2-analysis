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

func (s *Store) UnknownIDs(ctx context.Context, candidates []int64) ([]int64, error) {
	if len(candidates) == 0 {
		return nil, nil
	}
	rows, err := s.db.Query(ctx, `
		SELECT c.id FROM unnest($1::bigint[]) AS c(id)
		LEFT JOIN matches m ON m.match_id = c.id
		WHERE m.match_id IS NULL OR m.is_parsed = FALSE
	`, candidates)
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
			 JOIN pg_namespace n ON n.oid = c.relnamespace
			 WHERE n.nspname = 'public'
			   AND c.relkind = 'r'
			   AND c.relname IN ('matches', 'matches_default')
			   AND NOT EXISTS (SELECT 1 FROM pg_inherits WHERE inhrelid = c.oid)),
			(SELECT COALESCE(sum(reltuples::bigint), 0)
			 FROM pg_class c
			 JOIN pg_namespace n ON n.oid = c.relnamespace
			 WHERE n.nspname = 'public'
			   AND c.relkind = 'r'
			   AND c.relname IN ('player_matches', 'player_matches_default')
			   AND NOT EXISTS (SELECT 1 FROM pg_inherits WHERE inhrelid = c.oid))
	`).Scan(&c.Matches, &c.Players)
	return c, err
}

func (s *Store) IsIngested(ctx context.Context, matchID int64, startTime int64) (bool, error) {
	var exists bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM matches WHERE match_id = $1 AND start_time = $2)`, matchID, startTime).Scan(&exists)
	return exists, err
}

// checkDraftSchema computes the draft sequence hash and warns if it doesn't
// match any known schema. This is the canary that catches Valve draft format
// changes before they silently corrupt ML training data.
//
// Returns an error on mismatch so the caller can rollback the transaction
// and route the message to the DLQ — preventing corrupted data from reaching
// Postgres and the training pipeline.
func (s *Store) checkDraftSchema(ctx context.Context, m domain.Match) error {
	if len(m.PicksBans) == 0 {
		return nil
	}
	hash := computeSequenceHash(m.PicksBans)
	for _, known := range knownDraftSchemaHashes {
		if hash == known {
			return nil // known schema — all good
		}
	}
	err := fmt.Errorf("unknown draft schema: %s", hash)
	s.log.Warn("unknown draft schema detected — Valve may have changed the format",
		"match_id", int64(m.MatchID),
		"hash", hash,
		"num_picks_bans", len(m.PicksBans),
	)
	if s.m != nil {
		s.m.IngestFailure(ctx, metrics.KindDraftSchema, int64(m.MatchID), err)
	}
	return err
}