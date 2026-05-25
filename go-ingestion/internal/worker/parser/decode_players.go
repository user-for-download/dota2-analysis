package parser

import (
	"github.com/user-for-download/go-dota2/internal/storage/matchstore"
)

func decodePlayers(raw []rawPlayer) ([]matchstore.PlayerRow, []matchstore.PlayerDetailRow) {
	players := make([]matchstore.PlayerRow, 0, len(raw))
	details := make([]matchstore.PlayerDetailRow, 0, len(raw))
	for _, rp := range raw {
		players = append(players, convertPlayer(rp))
		details = append(details, convertPlayerDetail(rp))
	}
	return players, details
}

func convertPlayer(rp rawPlayer) matchstore.PlayerRow {
	win := false
	if rp.Win != nil {
		win = *rp.Win == 1
	}
	isRadiant := false
	if rp.IsRadiant != nil {
		isRadiant = *rp.IsRadiant
	}
	var firstblood bool
	if b := decToBool(rp.FirstbloodClaimed); b != nil {
		firstblood = *b
	}

	return matchstore.PlayerRow{
		PlayerSlot:              rp.PlayerSlot,
		AccountID:               deref64(rp.AccountID),
		HeroID:                  rp.HeroID,
		HeroVariant:             deref16(rp.HeroVariant),
		IsRadiant:               isRadiant,
		Win:                     win,
		PatchID:                 deref32(rp.PatchID),
		LobbyType:               deref16(rp.LobbyType),
		GameMode:                deref16(rp.GameMode),
		RankTier:                deref16(rp.RankTier),
		Kills:                   rp.Kills,
		Deaths:                  rp.Deaths,
		Assists:                 rp.Assists,
		Level:                   deref16(rp.Level),
		NetWorth:                deref32(rp.NetWorth),
		Gold:                    deref32(rp.Gold),
		GoldSpent:               deref32(rp.GoldSpent),
		GoldPerMin:              deref16(rp.GoldPerMin),
		XPPerMin:                deref16(rp.XPPerMin),
		LastHits:                deref16(rp.LastHits),
		Denies:                  deref16(rp.Denies),
		HeroDamage:              deref32(rp.HeroDamage),
		TowerDamage:             deref32(rp.TowerDamage),
		HeroHealing:             deref32(rp.HeroHealing),
		Item0:                   deref32(rp.Item0),
		Item1:                   deref32(rp.Item1),
		Item2:                   deref32(rp.Item2),
		Item3:                   deref32(rp.Item3),
		Item4:                   deref32(rp.Item4),
		Item5:                   deref32(rp.Item5),
		ItemNeutral:             deref32(rp.ItemNeutral),
		Backpack0:               deref32(rp.Backpack0),
		Backpack1:               deref32(rp.Backpack1),
		Backpack2:               deref32(rp.Backpack2),
		Backpack3:               deref32(rp.Backpack3),
		Lane:                    deref16(rp.Lane),
		LaneRole:                deref16(rp.LaneRole),
		IsRoaming:               derefBool(rp.IsRoaming),
		PartyID:                 deref32(rp.PartyID),
		PartySize:               deref16(rp.PartySize),
		Stuns:                   derefF32(rp.Stuns),
		ObsPlaced:               deref16(rp.ObsPlaced),
		SenPlaced:               deref16(rp.SenPlaced),
		CreepsStacked:           deref16(rp.CreepsStacked),
		CampsStacked:            deref16(rp.CampsStacked),
		RunePickups:             deref16(rp.RunePickups),
		FirstbloodClaimed:       firstblood,
		TeamfightParticipation:  derefF32(rp.TeamfightParticipation),
		TowersKilled:            deref16(rp.TowersKilled),
		RoshansKilled:           deref16(rp.RoshansKilled),
		ObserversPlaced:         deref16(rp.ObserversPlaced),
		LeaverStatus:            deref16(rp.LeaverStatus),
		GoldT:                   safeSlice(rp.GoldT),
		XPT:                     safeSlice(rp.XPT),
		LHT:                     safeSlice(rp.LHT),
		DNT:                     safeSlice(rp.DNT),
		Times:                   safeSlice(rp.Times),
		ThrowGold:               deref32(rp.ThrowGold),
		ComebackGold:            deref32(rp.ComebackGold),
		LossGold:                deref32(rp.LossGold),
		WinGold:                 deref32(rp.WinGold),
	}
}

func convertPlayerDetail(rp rawPlayer) matchstore.PlayerDetailRow {
	return matchstore.PlayerDetailRow{
		PlayerSlot:              rp.PlayerSlot,
		Damage:                  rp.Damage,
		DamageTaken:             rp.DamageTaken,
		DamageInflictor:         rp.DamageInflictor,
		DamageInflictorReceived: rp.DamageInflictorReceived,
		DamageTargets:           rp.DamageTargets,
		HeroHits:                rp.HeroHits,
		MaxHeroHit:              rp.MaxHeroHit,
		AbilityUses:             rp.AbilityUses,
		AbilityTargets:          rp.AbilityTargets,
		AbilityUpgradesArr:      rp.AbilityUpgradesArr,
		ItemUses:                rp.ItemUses,
		GoldReasons:             rp.GoldReasons,
		XPReasons:               rp.XPReasons,
		Killed:                  rp.Killed,
		KilledBy:                rp.KilledBy,
		KillStreaks:             rp.KillStreaks,
		MultiKills:              rp.MultiKills,
		LifeState:               rp.LifeState,
		LanePos:                 rp.LanePos,
		Obs:                     rp.Obs,
		Sen:                     rp.Sen,
		Actions:                 rp.Actions,
		Pings:                   rp.Pings,
		Runes:                   rp.Runes,
		Purchase:                rp.Purchase,
		ObsLog:                  rp.ObsLog,
		SenLog:                  rp.SenLog,
		ObsLeftLog:              rp.ObsLeftLog,
		SenLeftLog:              rp.SenLeftLog,
		PurchaseLog:             rp.PurchaseLog,
		KillsLog:                rp.KillsLog,
		BuybackLog:              rp.BuybackLog,
		RunesLog:                rp.RunesLog,
		ConnectionLog:           rp.ConnectionLog,
		PermanentBuffs:          rp.PermanentBuffs,
		NeutralTokensLog:        rp.NeutralTokensLog,
		NeutralItemHistory:      rp.NeutralItemHistory,
		AdditionalUnits:         rp.AdditionalUnits,
		Cosmetics:               rp.Cosmetics,
		Benchmarks:              rp.Benchmarks,
		AllWordCounts:           rp.AllWordCounts,
		MyWordCounts:            rp.MyWordCounts,
	}
}

func expandTimeseries(players []rawPlayer) []matchstore.TimeseriesRow {
	out := make([]matchstore.TimeseriesRow, 0, len(players)*60)
	for _, p := range players {
		maxMin := len(p.Times)
		if maxMin == 0 {
			if len(p.GoldT) > maxMin {
				maxMin = len(p.GoldT)
			}
			if len(p.XPT) > maxMin {
				maxMin = len(p.XPT)
			}
			if len(p.LHT) > maxMin {
				maxMin = len(p.LHT)
			}
			if len(p.DNT) > maxMin {
				maxMin = len(p.DNT)
			}
		}
		if maxMin == 0 {
			continue
		}
		for min := 0; min < maxMin; min++ {
			gold := safeIdx(p.GoldT, min)
			xp := safeIdx(p.XPT, min)
			lh := safeIdxSmall(p.LHT, min)
			dn := safeIdxSmall(p.DNT, min)
			if gold == nil && xp == nil && lh == nil && dn == nil {
				continue
			}
			out = append(out, matchstore.TimeseriesRow{
				PlayerSlot: p.PlayerSlot,
				Minute:     int16(min),
				HeroID:     p.HeroID,
				AccountID:  deref64(p.AccountID),
				PatchID:    deref32(p.PatchID),
				Gold:       deref32(gold),
				XP:         deref32(xp),
				LH:         deref16(lh),
				DN:         deref16(dn),
			})
		}
	}
	return out
}
