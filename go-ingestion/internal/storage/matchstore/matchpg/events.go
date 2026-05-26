package matchpg

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/user-for-download/dota2-analysis/go-core/domain"
)

func (s *Store) replaceObjectives(ctx context.Context, tx pgx.Tx, m domain.Match) error {
	if len(m.Objectives) == 0 {
		return nil
	}

	if _, err := tx.Exec(ctx, `SELECT 1 FROM matches WHERE match_id = $1 FOR UPDATE`, m.MatchID); err != nil {
		return fmt.Errorf("lock match: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM match_objectives WHERE match_id = $1`, m.MatchID,
	); err != nil {
		return fmt.Errorf("delete objectives: %w", err)
	}

	rows := make([][]any, 0, len(m.Objectives))
	for _, o := range m.Objectives {
		rows = append(rows, []any{
			int64(m.MatchID), m.StartTime, o.Time, o.Type,
			o.Slot, o.PlayerSlot, o.Team, nullIfStr(o.Key), o.Value, nullIfStr(o.Unit),
			jsonbOrNull([]byte(o.Raw)),
		})
	}

	cols := []string{
		"match_id", "start_time", "time", "type",
		"slot", "player_slot", "team", "key", "value", "unit", "raw",
	}

	if _, err := tx.CopyFrom(ctx,
		pgx.Identifier{"match_objectives"},
		cols,
		pgx.CopyFromRows(rows),
	); err != nil {
		return fmt.Errorf("copy objectives: %w", err)
	}
	return nil
}

func (s *Store) replaceChat(ctx context.Context, tx pgx.Tx, m domain.Match) error {
	if len(m.Chat) == 0 {
		return nil
	}

	if _, err := tx.Exec(ctx, `SELECT 1 FROM matches WHERE match_id = $1 FOR UPDATE`, m.MatchID); err != nil {
		return fmt.Errorf("lock match: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM match_chat WHERE match_id = $1`, m.MatchID,
	); err != nil {
		return fmt.Errorf("delete chat: %w", err)
	}

	rows := make([][]any, 0, len(m.Chat))
	for _, c := range m.Chat {
		rows = append(rows, []any{
			int64(m.MatchID), m.StartTime, c.Time, nullIfStr(c.Type), c.PlayerSlot, nullIfStr(c.Unit), nullIfStr(c.Key),
		})
	}

	cols := []string{
		"match_id", "start_time", "time", "type", "player_slot", "unit", "key",
	}

	if _, err := tx.CopyFrom(ctx,
		pgx.Identifier{"match_chat"},
		cols,
		pgx.CopyFromRows(rows),
	); err != nil {
		return fmt.Errorf("copy chat: %w", err)
	}
	return nil
}

func (s *Store) replaceTeamfights(ctx context.Context, tx pgx.Tx, m domain.Match) error {
	if len(m.Teamfights) == 0 {
		return nil
	}

	if _, err := tx.Exec(ctx, `SELECT 1 FROM matches WHERE match_id = $1 FOR UPDATE`, m.MatchID); err != nil {
		return fmt.Errorf("lock match: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM match_teamfights WHERE match_id = $1`, m.MatchID,
	); err != nil {
		return fmt.Errorf("delete teamfights: %w", err)
	}

	rows := make([][]any, 0, len(m.Teamfights))
	for _, tf := range m.Teamfights {
		rows = append(rows, []any{
			int64(m.MatchID), m.StartTime, tf.EndTime, tf.LastDeath, tf.Deaths,
			jsonbOrNull([]byte(tf.Players)),
		})
	}

	cols := []string{
		"match_id", "start_time", "end_time", "last_death", "deaths", "players",
	}

	if _, err := tx.CopyFrom(ctx,
		pgx.Identifier{"match_teamfights"},
		cols,
		pgx.CopyFromRows(rows),
	); err != nil {
		return fmt.Errorf("copy teamfights: %w", err)
	}
	return nil
}