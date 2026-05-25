package parser

import (
	"encoding/json"
	"fmt"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/storage/matchstore"
)

func decodeMatch(matchID int64, blob []byte) (matchstore.Match, error) {
	var rm rawMatch
	if err := json.Unmarshal(blob, &rm); err != nil {
		return matchstore.Match{}, fmt.Errorf("unmarshal: %w", err)
	}
	if rm.MatchID == 0 {
		rm.MatchID = matchID
	}
	if rm.MatchID <= 0 {
		return matchstore.Match{}, fmt.Errorf("invalid match_id: %d", rm.MatchID)
	}
	if rm.StartTime == 0 {
		return matchstore.Match{}, fmt.Errorf("match %d: start_time missing", rm.MatchID)
	}
	if rm.Duration < 0 {
		return matchstore.Match{}, fmt.Errorf("match %d: negative duration", rm.MatchID)
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
	m.Raw = blob
	return m, nil
}

func decodeMatchRoot(rm *rawMatch) matchstore.Match {
	return matchstore.Match{
		MatchIdentity: matchstore.MatchIdentity{
			MatchID:     rm.MatchID,
			MatchSeqNum: deref64(rm.MatchSeqNum),
			StartTime:   rm.StartTime,
			Duration:    rm.Duration,
			RadiantWin:  derefBool(rm.RadiantWin),
		},
		MatchBuildings: matchstore.MatchBuildings{
			TowerStatusRadiant:    deref16(rm.TowerStatusRadiant),
			TowerStatusDire:       deref16(rm.TowerStatusDire),
			BarracksStatusRadiant: deref16(rm.BarracksStatusRadiant),
			BarracksStatusDire:    deref16(rm.BarracksStatusDire),
			RadiantScore:          deref16(rm.RadiantScore),
			DireScore:             deref16(rm.DireScore),
		},
		MatchLobby: matchstore.MatchLobby{
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
		MatchContext: matchstore.MatchContext{
			PatchID:       deref32(rm.Patch),
			PositiveVotes: deref32(rm.PositiveVotes),
			NegativeVotes: deref32(rm.NegativeVotes),
			LeagueID:      deref32(rm.LeagueID),
			SeriesID:      deref32(rm.SeriesID),
			SeriesType:    deref16(rm.SeriesType),
		},
		MatchTeams: matchstore.MatchTeams{
			RadiantTeamID:  deref64(rm.RadiantTeamID),
			DireTeamID:     deref64(rm.DireTeamID),
			RadiantCaptain: deref64(rm.RadiantCaptain),
			DireCaptain:    deref64(rm.DireCaptain),
		},
		MatchReplay: matchstore.MatchReplay{
			ReplaySalt: deref64(rm.ReplaySalt),
			ReplayURL:  derefStr(rm.ReplayURL),
			Pauses:     rm.Pauses,
		},
		IsParsed: isMatchParsed(*rm),
	}
}

func validate(m matchstore.Match) error {
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

func isMatchParsed(rm rawMatch) bool {
	if len(rm.Players) == 0 {
		return false
	}
	for _, p := range rm.Players {
		if p.PurchaseLog != nil || len(p.GoldT) > 0 || len(p.XPT) > 0 {
			return true
		}
	}
	return false
}
