package matchpg

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/user-for-download/go-dota2/internal/storage/matchstore"
)

func (s *Store) upsertPicksBans(ctx context.Context, tx pgx.Tx, m matchstore.Match) error {
	var rows [][]any
	for _, pb := range m.PicksBans {
		rows = append(rows, []any{m.MatchID, pb.Order, pb.IsPick, pb.HeroID, pb.Team})
	}

	cols := []string{"match_id", "ord", "is_pick", "hero_id", "team"}

	return bulkUpsert(ctx, tx, "_stage_picks_bans", "picks_bans", cols, "ON CONFLICT (match_id, ord) DO NOTHING", rows)
}

func (s *Store) upsertDraftTimings(ctx context.Context, tx pgx.Tx, m matchstore.Match) error {
	var rows [][]any
	for _, dt := range m.DraftTimings {
		rows = append(rows, []any{
			m.MatchID, dt.Order, dt.Pick, dt.ActiveTeam, nullIf0_16(dt.HeroID), dt.PlayerSlot, dt.ExtraTime, dt.TotalTimeTaken,
		})
	}

	cols := []string{
		"match_id", "ord", "pick", "active_team", "hero_id", "player_slot", "extra_time", "total_time_taken",
	}

	return bulkUpsert(ctx, tx, "_stage_draft_timings", "draft_timings", cols, "ON CONFLICT (match_id, ord) DO NOTHING", rows)
}