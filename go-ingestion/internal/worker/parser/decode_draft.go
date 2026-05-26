package parser

import "github.com/user-for-download/dota2-analysis/go-core/domain"

func decodePicksBans(raw []rawPickBan) []domain.PickBan {
	if len(raw) == 0 {
		return nil
	}
	rows := make([]domain.PickBan, 0, len(raw))
	for _, pb := range raw {
		rows = append(rows, domain.PickBan{
			Order:  pb.Order,
			IsPick: pb.IsPick,
			HeroID: domain.HeroID(pb.HeroID),
			Team:   pb.Team,
		})
	}
	return rows
}

func decodeDraftTimings(raw []rawDraftTiming) []domain.DraftTiming {
	if len(raw) == 0 {
		return nil
	}
	rows := make([]domain.DraftTiming, 0, len(raw))
	for _, d := range raw {
		rows = append(rows, domain.DraftTiming{
			Order:          d.Order,
			Pick:           d.Pick,
			ActiveTeam:     deref16(d.ActiveTeam),
			HeroID:         domain.HeroID(deref16(d.HeroID)),
			PlayerSlot:     deref16(d.PlayerSlot),
			ExtraTime:      deref32(d.ExtraTime),
			TotalTimeTaken: deref32(d.TotalTimeTaken),
		})
	}
	return rows
}
