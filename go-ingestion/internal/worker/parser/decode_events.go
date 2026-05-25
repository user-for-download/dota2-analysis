package parser

import (
	"encoding/json"
	"strconv"

	"github.com/user-for-download/go-dota2/internal/storage/matchstore"
)

func decodeObjectives(raw []rawObjective) []matchstore.ObjectiveRow {
	if len(raw) == 0 {
		return nil
	}
	rows := make([]matchstore.ObjectiveRow, 0, len(raw))
	for _, o := range raw {
		rawJSON, _ := json.Marshal(o)
		keyStr := ""
		if k := objectiveKeyAsString(o.Key); k != nil {
			keyStr = *k
		}
		rows = append(rows, matchstore.ObjectiveRow{
			Time:       o.Time,
			Type:       o.Type,
			Slot:       deref16(o.Slot),
			PlayerSlot: deref16(o.PlayerSlot),
			Team:       deref16(o.Team),
			Key:        keyStr,
			Value:      deref32(o.Value),
			Unit:       derefStr(o.Unit),
			Raw:        rawJSON,
		})
	}
	return rows
}

func decodeChat(raw []rawChat) []matchstore.ChatRow {
	if len(raw) == 0 {
		return nil
	}
	rows := make([]matchstore.ChatRow, 0, len(raw))
	for _, c := range raw {
		rows = append(rows, matchstore.ChatRow{
			Time:       c.Time,
			Type:       derefStr(c.Type),
			PlayerSlot: deref16(c.PlayerSlot),
			Unit:       derefStr(c.Unit),
			Key:        derefStr(c.Key),
		})
	}
	return rows
}

func decodeTeamfights(raw []rawTeamfight) []matchstore.TeamfightRow {
	if len(raw) == 0 {
		return nil
	}
	rows := make([]matchstore.TeamfightRow, 0, len(raw))
	for _, t := range raw {
		rows = append(rows, matchstore.TeamfightRow{
			EndTime:   t.End,
			LastDeath: deref32(t.LastDeath),
			Deaths:    deref16(t.Deaths),
			Players:   t.Players,
		})
	}
	return rows
}

func decodeAdvantages(gold, xp []int32) *matchstore.AdvantagesRow {
	if len(gold) == 0 && len(xp) == 0 {
		return nil
	}
	return &matchstore.AdvantagesRow{
		RadiantGoldAdv: gold,
		RadiantXPAdv:   xp,
	}
}

func objectiveKeyAsString(raw json.RawMessage) *string {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return &s
	}
	var n int
	if err := json.Unmarshal(raw, &n); err == nil {
		s = strconv.Itoa(n)
		return &s
	}
	return nil
}
