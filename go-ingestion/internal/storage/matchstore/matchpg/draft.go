package matchpg

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/user-for-download/dota2-analysis/go-core/domain"
)

// knownDraftSchemaHashes contains SHA256 hashes of known draft sequences,
// keyed by a human-readable label. When Valve changes the draft format,
// the hash will stop matching and the canary fires.
var knownDraftSchemaHashes = map[string]string{
	// cm_734: Captain's Mode post-7.33+ format (24 slots).
	// Sequence: R:ban,R:ban,D:ban,D:ban,R:ban,D:ban,D:ban,R:pick,D:pick,
	//           R:ban,R:ban,D:ban,D:pick,R:pick,R:pick,D:pick,D:pick,R:pick,
	//           R:ban,D:ban,R:ban,D:ban,R:pick,D:pick
	"cm_734": "fe4b4a1ff4075353c546a837130bcdd9d6b25c9d5deeb9a8e9f495b17ce338f2",
}

// computeSequenceHash returns the SHA256 hex digest of the draft sequence.
// Each pick/ban is encoded as "team:action|" so reordering or format changes
// produce a different hash.
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