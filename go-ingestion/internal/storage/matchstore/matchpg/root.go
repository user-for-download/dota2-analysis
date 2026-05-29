package matchpg

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/user-for-download/dota2-analysis/go-core/domain"
)

func (s *Store) upsertMatchRoot(ctx context.Context, tx pgx.Tx, m domain.Match) (bool, error) {
	const q = `
		INSERT INTO matches (
			match_id, match_seq_num, start_time, duration, radiant_win,
			tower_status_radiant, tower_status_dire, barracks_status_radiant, barracks_status_dire,
			radiant_score, dire_score, first_blood_time, lobby_type, game_mode, cluster, region, skill, engine,
			human_players, version, patch_id, positive_votes, negative_votes,
			leagueid, series_id, series_type, radiant_team_id, dire_team_id, radiant_captain, dire_captain,
			replay_salt, replay_url, pauses, is_parsed, created_at, updated_at
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,
			$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32,
			$33::jsonb, $34, NOW(), NOW()
		) ON CONFLICT (match_id, start_time) DO UPDATE SET
			is_parsed = matches.is_parsed OR EXCLUDED.is_parsed,
			radiant_win = COALESCE(EXCLUDED.radiant_win, matches.radiant_win),
			tower_status_radiant = COALESCE(EXCLUDED.tower_status_radiant, matches.tower_status_radiant),
			tower_status_dire = COALESCE(EXCLUDED.tower_status_dire, matches.tower_status_dire),
			barracks_status_radiant = COALESCE(EXCLUDED.barracks_status_radiant, matches.barracks_status_radiant),
			barracks_status_dire = COALESCE(EXCLUDED.barracks_status_dire, matches.barracks_status_dire),
			radiant_score = COALESCE(EXCLUDED.radiant_score, matches.radiant_score),
			dire_score = COALESCE(EXCLUDED.dire_score, matches.dire_score),
			first_blood_time = COALESCE(EXCLUDED.first_blood_time, matches.first_blood_time),
			replay_salt = COALESCE(EXCLUDED.replay_salt, matches.replay_salt),
			replay_url = COALESCE(EXCLUDED.replay_url, matches.replay_url),
			pauses = COALESCE(EXCLUDED.pauses, matches.pauses),
			updated_at = NOW()
		WHERE matches.is_parsed = FALSE OR EXCLUDED.is_parsed = TRUE`

	res, err := tx.Exec(ctx, q,
		int64(m.MatchID), nullIf0_64(m.MatchSeqNum), m.StartTime, m.Duration, m.RadiantWin,
		m.TowerStatusRadiant, m.TowerStatusDire, m.BarracksStatusRadiant, m.BarracksStatusDire,
		m.RadiantScore, m.DireScore, m.FirstBloodTime, m.LobbyType, m.GameMode, m.Cluster, m.Region, m.Skill, m.Engine,
		m.HumanPlayers, m.Version, int32(m.PatchID), m.PositiveVotes, m.NegativeVotes,
		m.LeagueID, m.SeriesID, m.SeriesType, int64(m.RadiantTeamID), int64(m.DireTeamID), int64(m.RadiantCaptain), int64(m.DireCaptain),
		int64(m.ReplaySalt), nullIfStr(m.ReplayURL), jsonbOrNull([]byte(m.Pauses)), m.IsParsed,
	)
	if err != nil {
		return false, fmt.Errorf("insert matches: %w", err)
	}
	n := res.RowsAffected()
	return n > 0, nil
}

func (s *Store) upsertTeamMatches(ctx context.Context, tx pgx.Tx, m domain.Match) error {
	if m.RadiantTeamID == 0 && m.DireTeamID == 0 {
		return nil
	}

	const q = `
		INSERT INTO team_matches (
			team_id, match_id, start_time, is_radiant, win, leagueid
		) VALUES (
			$1, $2, $3, $4, $5, $6
		) ON CONFLICT (team_id, match_id) DO UPDATE SET
			start_time = EXCLUDED.start_time,
			is_radiant = EXCLUDED.is_radiant,
			win = EXCLUDED.win,
			leagueid = EXCLUDED.leagueid
	`

	if m.RadiantTeamID != 0 {
		_, err := tx.Exec(ctx, q,
			int64(m.RadiantTeamID), int64(m.MatchID), m.StartTime, true, m.RadiantWin, nullIf0_32(m.LeagueID),
		)
		if err != nil {
			return fmt.Errorf("insert team_matches radiant: %w", err)
		}
	}

	if m.DireTeamID != 0 {
		_, err := tx.Exec(ctx, q,
			int64(m.DireTeamID), int64(m.MatchID), m.StartTime, false, negateBoolPtr(m.RadiantWin), nullIf0_32(m.LeagueID),
		)
		if err != nil {
			return fmt.Errorf("insert team_matches dire: %w", err)
		}
	}

	return nil
}

func (s *Store) upsertAdvantages(ctx context.Context, tx pgx.Tx, m domain.Match) error {
	if m.Advantages == nil {
		return nil
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO match_advantages (match_id, radiant_gold_adv, radiant_xp_adv) VALUES ($1, $2, $3)
		ON CONFLICT (match_id) DO UPDATE SET radiant_gold_adv = EXCLUDED.radiant_gold_adv, radiant_xp_adv = EXCLUDED.radiant_xp_adv
	`, int64(m.MatchID), m.Advantages.RadiantGoldAdv, m.Advantages.RadiantXPAdv)
	return err
}

func (s *Store) upsertCosmetics(ctx context.Context, tx pgx.Tx, m domain.Match) error {
	if len(m.Cosmetics) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO match_cosmetics (match_id, cosmetics) VALUES ($1, $2::jsonb)
		ON CONFLICT (match_id) DO UPDATE SET cosmetics = EXCLUDED.cosmetics
	`, int64(m.MatchID), string(m.Cosmetics))
	return err
}