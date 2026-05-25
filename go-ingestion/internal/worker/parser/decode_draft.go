package parser

import "github.com/user-for-download/dota2-analysis/go-ingestion/internal/storage/matchstore"

func decodePicksBans(raw []rawPickBan) []matchstore.PickBanRow {
	if len(raw) == 0 {
		return nil
	}
	rows := make([]matchstore.PickBanRow, 0, len(raw))
	for _, pb := range raw {
		rows = append(rows, matchstore.PickBanRow{
			Order:  pb.Order,
			IsPick: pb.IsPick,
			HeroID: pb.HeroID,
			Team:   pb.Team,
		})
	}
	return rows
}

func decodeDraftTimings(raw []rawDraftTiming) []matchstore.DraftTimingRow {
	if len(raw) == 0 {
		return nil
	}
	rows := make([]matchstore.DraftTimingRow, 0, len(raw))
	for _, d := range raw {
		rows = append(rows, matchstore.DraftTimingRow{
			Order:          d.Order,
			Pick:           d.Pick,
			ActiveTeam:     deref16(d.ActiveTeam),
			HeroID:         deref16(d.HeroID),
			PlayerSlot:     deref16(d.PlayerSlot),
			ExtraTime:      deref32(d.ExtraTime),
			TotalTimeTaken: deref32(d.TotalTimeTaken),
		})
	}
	return rows
}
