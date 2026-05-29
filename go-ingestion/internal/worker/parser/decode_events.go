package parser

import (
	"encoding/json"
	"strconv"

	"github.com/user-for-download/dota2-analysis/go-core/domain"
)

func decodeObjectives(raw []json.RawMessage) []domain.Objective {
	if len(raw) == 0 {
		return nil
	}
	rows := make([]domain.Objective, 0, len(raw))
	for _, rawMsg := range raw {
		var o rawObjective
		if err := json.Unmarshal(rawMsg, &o); err != nil {
			continue
		}
		keyStr := ""
		if k := objectiveKeyAsString(o.Key); k != nil {
			keyStr = *k
		}
		rows = append(rows, domain.Objective{
			Time:       o.Time,
			Type:       o.Type,
			Slot:       deref16(o.Slot),
			PlayerSlot: deref16(o.PlayerSlot),
			Team:       deref16(o.Team),
			Key:        keyStr,
			Value:      deref32(o.Value),
			Unit:       derefStr(o.Unit),
			Raw:        rawMsg, // original JSON from the API response, no re-marshal
		})
	}
	return rows
}

func decodeChat(raw []rawChat) []domain.Chat {
	if len(raw) == 0 {
		return nil
	}
	rows := make([]domain.Chat, 0, len(raw))
	for _, c := range raw {
		rows = append(rows, domain.Chat{
			Time:       c.Time,
			Type:       derefStr(c.Type),
			PlayerSlot: deref16(c.PlayerSlot),
			Unit:       derefStr(c.Unit),
			Key:        derefStr(c.Key),
		})
	}
	return rows
}

func decodeTeamfights(raw []rawTeamfight) []domain.Teamfight {
	if len(raw) == 0 {
		return nil
	}
	rows := make([]domain.Teamfight, 0, len(raw))
	for _, t := range raw {
		rows = append(rows, domain.Teamfight{
			EndTime:   t.End,
			LastDeath: deref32(t.LastDeath),
			Deaths:    deref16(t.Deaths),
			Players:   t.Players,
		})
	}
	return rows
}

func decodeAdvantages(gold, xp []int32) *domain.Advantages {
	if len(gold) == 0 && len(xp) == 0 {
		return nil
	}
	return &domain.Advantages{
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
