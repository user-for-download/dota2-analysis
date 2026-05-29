package matchpg

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/user-for-download/dota2-analysis/go-core/domain"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/metrics"
)

// knownDraftSchemaHashes is the compile-time seed — only used for patches
// that predate the DB or when LearnDraftSchemas has not yet been called.
// Add new entries here only if LearnDraftSchemas cannot cover them.
var knownDraftSchemaHashes = map[string]string{
	// cm_734: Captain's Mode post-7.33+ format (24 slots).
	// Sequence: D:ban,R:ban,R:ban,D:ban,R:ban,D:ban,D:ban,D:pick,R:pick,
	//           D:ban,D:ban,R:ban,R:pick,D:pick,D:pick,R:pick,R:pick,D:pick,
	//           D:ban,R:ban,D:ban,R:ban,D:pick,R:pick
	"cm_734": "fe4b4a1ff4075353c546a837130bcdd9d6b25c9d5deeb9a8e9f495b17ce338f2",

	// cm_739_radiant_fp: Captain's Mode format introduced in 7.39 (Radiant First Pick)
	// Sequence: R:ban,R:ban,D:ban,D:ban,R:ban,D:ban,D:ban,R:pick,D:pick...
	"cm_739_radiant_fp": "d434316799f4a577576e38f3c16a2e2bb5bf4894e6763b45b86a56ac60c0a14d",

	// cm_739_dire_fp: Captain's Mode format introduced in 7.39 (Dire First Pick)
	// Sequence: D:ban,D:ban,R:ban,R:ban,D:ban,R:ban,R:ban,D:pick,R:pick...
	"cm_739_dire_fp": "1784e8a6bd078c8013253bf3e9dc3a01b3950a3103ab52b4f97d9af0d60a2af9",
}

// draftSchemas holds the live set of accepted hashes — seed + learned.
var draftSchemas struct {
	mu     sync.RWMutex
	hashes map[string]struct{} // set of accepted hash strings
}

func init() {
	draftSchemas.hashes = make(map[string]struct{})
	for _, h := range knownDraftSchemaHashes {
		draftSchemas.hashes[h] = struct{}{}
	}
}

// LearnDraftSchemas queries the DB for one Captain's Mode match per recent
// patch, computes its draft sequence hash, and registers any new hashes.
// Safe to call at startup.
//
// This eliminates manual hash updates when Valve changes the CM draft format:
// as long as one match from the new format is already ingested (which happens
// the first time a match slips through before the canary fires, or via a
// seeded patch), all subsequent matches will be accepted.
//
// The query uses DISTINCT ON (patch_id) to pick one representative parsed CM
// match per patch_id (the most recent match for that patch), staying efficient
// without a full table scan.
func LearnDraftSchemas(ctx context.Context, db *pgxpool.Pool, log *slog.Logger) error {
	const q = `
		SELECT DISTINCT ON (m.patch_id)
		    m.match_id,
		    m.patch_id
		FROM public.matches m
		WHERE m.game_mode  = 2            -- Captain's Mode only
		  AND m.is_parsed  = TRUE
		  AND m.start_time >= EXTRACT(EPOCH FROM NOW() - INTERVAL '180 days')::BIGINT
		ORDER BY m.patch_id DESC, m.match_id DESC
	`
	rows, err := db.Query(ctx, q)
	if err != nil {
		return fmt.Errorf("learn draft schemas: query: %w", err)
	}
	defer rows.Close()

	type matchMeta struct {
		matchID int64
		patchID int32
	}
	var matches []matchMeta
	for rows.Next() {
		var mm matchMeta
		if err := rows.Scan(&mm.matchID, &mm.patchID); err != nil {
			return fmt.Errorf("learn draft schemas: scan: %w", err)
		}
		matches = append(matches, mm)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("learn draft schemas: rows: %w", err)
	}

	if len(matches) == 0 {
		log.Info("learn draft schemas: no CM matches found in DB, using seed hashes only")
		return nil
	}

	newHashes := 0
	for _, mm := range matches {
		hash, err := hashMatchFromDB(ctx, db, mm.matchID)
		if err != nil {
			log.Warn("learn draft schemas: could not hash match",
				"match_id", mm.matchID, "patch_id", mm.patchID, "err", err)
			continue
		}

		draftSchemas.mu.Lock()
		if _, exists := draftSchemas.hashes[hash]; !exists {
			draftSchemas.hashes[hash] = struct{}{}
			newHashes++
			log.Info("learn draft schemas: registered new hash",
				"hash", hash[:16]+"...", "patch_id", mm.patchID, "match_id", mm.matchID)
		}
		draftSchemas.mu.Unlock()
	}

	{
		draftSchemas.mu.RLock()
		total := len(draftSchemas.hashes)
		draftSchemas.mu.RUnlock()
		log.Info("learn draft schemas: complete",
			"patches_sampled", len(matches),
			"new_hashes", newHashes,
			"total_known", total,
		)
	}
	return nil
}

// hashMatchFromDB fetches all picks_bans for a match ordered by ord and
// computes the sequence hash. Same algorithm as computeSequenceHash.
func hashMatchFromDB(ctx context.Context, db *pgxpool.Pool, matchID int64) (string, error) {
	rows, err := db.Query(ctx,
		`SELECT is_pick, team FROM public.picks_bans WHERE match_id = $1 ORDER BY ord ASC`,
		matchID,
	)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	h := sha256.New()
	n := 0
	for rows.Next() {
		var isPick bool
		var team int16
		if err := rows.Scan(&isPick, &team); err != nil {
			return "", err
		}
		action := "ban"
		if isPick {
			action = "pick"
		}
		h.Write([]byte(fmt.Sprintf("%d:%s|", team, action)))
		n++
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if n == 0 {
		return "", fmt.Errorf("no picks_bans rows for match %d", matchID)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// computeSequenceHash returns the SHA256 hex digest of the draft sequence.
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

// checkDraftSchema validates the draft sequence hash for Captain's Mode
// matches (game_mode=2). This is the canary that catches Valve draft format
// changes before they silently corrupt ML training data.
//
// For non-CM game modes (All Pick, Turbo, etc.) the check is skipped —
// those modes have different or no draft sequences.
//
// Returns an error on mismatch so the caller can rollback the transaction
// and route the message to the DLQ — preventing corrupted data from reaching
// Postgres and the training pipeline.
func (s *Store) checkDraftSchema(ctx context.Context, m domain.Match) error {
	if m.GameMode != 2 {
		return nil
	}
	if len(m.PicksBans) == 0 {
		return nil
	}
	hash := computeSequenceHash(m.PicksBans)

	draftSchemas.mu.RLock()
	_, known := draftSchemas.hashes[hash]
	draftSchemas.mu.RUnlock()

	if known {
		return nil
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
