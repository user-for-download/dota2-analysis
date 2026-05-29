package parser

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/user-for-download/dota2-analysis/go-core/domain"
)

func decodeMatch(matchID int64, blob []byte) (domain.Match, error) {
	var rm rawMatch
	if err := json.Unmarshal(blob, &rm); err != nil {
		return domain.Match{}, fmt.Errorf("unmarshal: %w", err)
	}
	if rm.MatchID == 0 {
		rm.MatchID = matchID
	}
	if rm.MatchID <= 0 {
		return domain.Match{}, fmt.Errorf("invalid match_id: %d", rm.MatchID)
	}
	if rm.StartTime == 0 {
		return domain.Match{}, fmt.Errorf("match %d: start_time missing", rm.MatchID)
	}
	if rm.Duration < 0 {
		return domain.Match{}, fmt.Errorf("match %d: negative duration", rm.MatchID)
	}

	m := decodeMatchRoot(&rm)
	m.Players, m.Details = decodePlayers(rm.Players)
	m.PicksBans = decodePicksBans(rm.PicksBans)
	m.DraftTimings = decodeDraftTimings(rm.DraftTimings)
	m.Objectives = decodeObjectives(rm.Objectives)
	m.Chat = decodeChat(rm.Chat)
	m.Teamfights = decodeTeamfights(rm.Teamfights)
	m.Advantages = decodeAdvantages(rm.RadiantGoldAdv, rm.RadiantXPAdv)
	m.Cosmetics = rm.Cosmetics
	m.Timeseries = expandTimeseries(rm.Players)
	m.Raw = json.RawMessage(blob)
	return m, nil
}

func decodeMatchRoot(rm *rawMatch) domain.Match {
	return domain.Match{
		MatchIdentity: domain.MatchIdentity{
			MatchID:     domain.MatchID(rm.MatchID),
			MatchSeqNum: deref64(rm.MatchSeqNum),
			StartTime:   rm.StartTime,
			Duration:    rm.Duration,
			RadiantWin:  rm.RadiantWin,
		},
		MatchBuildings: domain.MatchBuildings{
			TowerStatusRadiant:    deref16(rm.TowerStatusRadiant),
			TowerStatusDire:       deref16(rm.TowerStatusDire),
			BarracksStatusRadiant: deref16(rm.BarracksStatusRadiant),
			BarracksStatusDire:    deref16(rm.BarracksStatusDire),
			RadiantScore:          deref16(rm.RadiantScore),
			DireScore:             deref16(rm.DireScore),
		},
		MatchLobby: domain.MatchLobby{
			FirstBloodTime: deref32(rm.FirstBloodTime),
			LobbyType:      deref16(rm.LobbyType),
			GameMode:       deref16(rm.GameMode),
			Cluster:        deref16(rm.Cluster),
			Region:         deref16(rm.Region),
			Skill:          deref16(rm.Skill),
			Engine:         deref16(rm.Engine),
			HumanPlayers:   deref16(rm.HumanPlayers),
			Version:        deref16(rm.Version),
		},
		MatchContext: domain.MatchContext{
			PatchID:       domain.PatchID(deref32(rm.Patch)),
			PositiveVotes: deref32(rm.PositiveVotes),
			NegativeVotes: deref32(rm.NegativeVotes),
			LeagueID:      deref32(rm.LeagueID),
			SeriesID:      deref32(rm.SeriesID),
			SeriesType:    deref16(rm.SeriesType),
		},
		MatchTeams: domain.MatchTeams{
			RadiantTeamID:  domain.TeamID(deref64(rm.RadiantTeamID)),
			DireTeamID:     domain.TeamID(deref64(rm.DireTeamID)),
			RadiantCaptain: domain.AccountID(deref64(rm.RadiantCaptain)),
			DireCaptain:    domain.AccountID(deref64(rm.DireCaptain)),
		},
		MatchReplay: domain.MatchReplay{
			ReplaySalt: deref64(rm.ReplaySalt),
			ReplayURL:  derefStr(rm.ReplayURL),
			Pauses:     rm.Pauses,
		},
		IsParsed: isMatchParsed(*rm, rm.MatchID),
	}
}

func validate(m domain.Match) error {
	if m.MatchID <= 0 {
		return fmt.Errorf("invalid match_id: %d", m.MatchID)
	}
	if m.StartTime <= 0 {
		return fmt.Errorf("match %d: start_time required", m.MatchID)
	}
	for _, p := range m.Players {
		if !validPlayerSlot(p.PlayerSlot) {
			return fmt.Errorf("match %d: invalid player_slot %d", m.MatchID, p.PlayerSlot)
		}
		if p.HeroID < 0 {
			return fmt.Errorf("match %d slot %d: invalid hero_id %d", m.MatchID, p.PlayerSlot, p.HeroID)
		}
	}
	for _, pb := range m.PicksBans {
		if pb.Team != 0 && pb.Team != 1 {
			return fmt.Errorf("match %d: picks_bans team must be 0|1 (got %d)", m.MatchID, pb.Team)
		}
	}
	return nil
}

func validPlayerSlot(s int16) bool {
	return (s >= 0 && s <= 4) || (s >= 128 && s <= 132)
}

func isMatchParsed(rm rawMatch, matchID int64) bool {
	if len(rm.Players) == 0 {
		slog.Debug("match unparsed: no players array", "match_id", matchID)
		return false
	}
	for _, p := range rm.Players {
		// json.RawMessage stores JSON null as []byte("null"), not nil.
		hasLogs := len(p.PurchaseLog) > 0 && string(p.PurchaseLog) != "null"
		if hasLogs || len(p.GoldT) > 0 || len(p.XPT) > 0 {
			return true
		}
	}
	// Log exactly what is missing from the first player to prove it's unparsed
	p0 := rm.Players[0]
	slog.Debug("match unparsed: missing replay data from upstream API",
		"match_id", matchID,
		"players_count", len(rm.Players),
		"p0_gold_t_len", len(p0.GoldT),
		"p0_xp_t_len", len(p0.XPT),
		"p0_has_purchase_log", len(p0.PurchaseLog) > 0 && string(p0.PurchaseLog) != "null",
		"p0_has_ability_uses", p0.AbilityUses != nil,
	)
	return false
}
