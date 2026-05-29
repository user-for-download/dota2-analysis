package matchpg

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/user-for-download/dota2-analysis/go-core/domain"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/metrics"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/worker/parser"
)

// computeSequenceHash returns the SHA256 hex digest of the draft sequence.
// Retained for monitoring purposes only — validation uses structural pattern
// matching (parser.ValidateDraft) which handles declined bans gracefully.
func computeSequenceHash(pbs []domain.PickBan) string {
	h := sha256.New()
	for _, pb := range pbs {
		action := "ban"
		if pb.IsPick {
			action = "pick"
		}
		h.Write([]byte(fmt.Sprintf("%d:%s|", pb.Team, action)))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// checkDraftSchema validates the draft structure for Captain's Mode matches
// (game_mode=2). Uses structural pattern matching against known CM phase
// sequences, allowing for declined bans (incomplete ban phases).
//
// For non-CM game modes (All Pick, Turbo, etc.) the check is skipped —
// those modes have different or no draft sequences.
//
// Returns an error on structural mismatch so the caller can rollback the
// transaction and route the message to the DLQ. The hash is computed and
// logged for monitoring but is NOT used for rejection — hash-matching is
// fundamentally unable to handle the ~16K possible incomplete-draft hashes
// created by declined bans.
func (s *Store) checkDraftSchema(ctx context.Context, m domain.Match) error {
	if m.GameMode != 2 {
		return nil
	}
	if len(m.PicksBans) == 0 {
		return nil
	}

	// Log the hash for monitoring — this helps detect genuinely new formats
	// that may need pattern table updates.
	hash := computeSequenceHash(m.PicksBans)
	s.log.Debug("draft schema observed",
		"match_id", int64(m.MatchID),
		"hash", hash,
		"num_picks_bans", len(m.PicksBans),
	)

	// Structural validation handles all valid CM drafts including those
	// with declined bans (which produce infinite unique hashes).
	if err := parser.ValidateDraft(m.PicksBans); err != nil {
		s.log.Warn("invalid draft structure",
			"match_id", int64(m.MatchID),
			"hash", hash,
			"num_picks_bans", len(m.PicksBans),
			"err", err,
		)
		if s.m != nil {
			s.m.IngestFailure(ctx, metrics.KindDraftSchema, int64(m.MatchID), err)
		}
		return fmt.Errorf("invalid draft: %w", err)
	}

	// Detect and log the draft era for monitoring.
	era := parser.DetectDraftEra(m.PicksBans)
	s.log.Debug("draft era detected",
		"match_id", int64(m.MatchID),
		"era", era,
		"hash", hash,
		"num_picks_bans", len(m.PicksBans),
	)
	if era == "unknown" {
		s.log.Warn("unknown draft era",
			"match_id", int64(m.MatchID),
			"hash", hash,
			"num_picks_bans", len(m.PicksBans),
		)
	}

	return nil
}

func (s *Store) upsertPicksBans(ctx context.Context, tx pgx.Tx, m domain.Match) error {
	var rows [][]any
	for _, pb := range m.PicksBans {
		rows = append(rows, []any{int64(m.MatchID), pb.Order, pb.IsPick, int16(pb.HeroID), pb.Team})
	}

	cols := []string{"match_id", "ord", "is_pick", "hero_id", "team"}

	return bulkUpsert(ctx, tx, "_stage_picks_bans", "picks_bans", cols, "ON CONFLICT (match_id, ord) DO NOTHING", rows)
}

func (s *Store) upsertDraftTimings(ctx context.Context, tx pgx.Tx, m domain.Match) error {
	var rows [][]any
	for _, dt := range m.DraftTimings {
		rows = append(rows, []any{
			int64(m.MatchID), dt.Order, dt.Pick, dt.ActiveTeam, nullIf0_16(int16(dt.HeroID)), dt.PlayerSlot, dt.ExtraTime, dt.TotalTimeTaken,
		})
	}

	cols := []string{
		"match_id", "ord", "pick", "active_team", "hero_id", "player_slot", "extra_time", "total_time_taken",
	}

	return bulkUpsert(ctx, tx, "_stage_draft_timings", "draft_timings", cols, "ON CONFLICT (match_id, ord) DO NOTHING", rows)
}
