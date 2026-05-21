package parser

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"

	"github.com/user-for-download/go-dota2/internal/storage/matchstore"
)

type Match = matchstore.Match

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
		MatchID:               rm.MatchID,
		MatchSeqNum:           deref64(rm.MatchSeqNum),
		StartTime:             rm.StartTime,
		Duration:              rm.Duration,
		RadiantWin:            derefBool(rm.RadiantWin),
		TowerStatusRadiant:    deref16(rm.TowerStatusRadiant),
		TowerStatusDire:       deref16(rm.TowerStatusDire),
		BarracksStatusRadiant: deref16(rm.BarracksStatusRadiant),
		BarracksStatusDire:    deref16(rm.BarracksStatusDire),
		RadiantScore:          deref16(rm.RadiantScore),
		DireScore:             deref16(rm.DireScore),
		FirstBloodTime:        deref32(rm.FirstBloodTime),
		LobbyType:             deref16(rm.LobbyType),
		GameMode:              deref16(rm.GameMode),
		Cluster:               deref16(rm.Cluster),
		Region:                deref16(rm.Region),
		Skill:                 deref16(rm.Skill),
		Engine:                deref16(rm.Engine),
		HumanPlayers:          deref16(rm.HumanPlayers),
		Version:               deref16(rm.Version),
		PatchID:               deref32(rm.Patch),
		PositiveVotes:         deref32(rm.PositiveVotes),
		NegativeVotes:         deref32(rm.NegativeVotes),
		LeagueID:              deref32(rm.LeagueID),
		SeriesID:              deref32(rm.SeriesID),
		SeriesType:            deref16(rm.SeriesType),
		RadiantTeamID:         deref64(rm.RadiantTeamID),
		DireTeamID:            deref64(rm.DireTeamID),
		RadiantCaptain:        deref64(rm.RadiantCaptain),
		DireCaptain:           deref64(rm.DireCaptain),
		ReplaySalt:            deref64(rm.ReplaySalt),
		ReplayURL:             derefStr(rm.ReplayURL),
		Pauses:                rm.Pauses,
		IsParsed:              isMatchParsed(*rm),
	}
}

func decodePlayers(raw []rawPlayer) ([]matchstore.PlayerRow, []matchstore.PlayerDetailRow) {
	players := make([]matchstore.PlayerRow, 0, len(raw))
	details := make([]matchstore.PlayerDetailRow, 0, len(raw))
	for _, rp := range raw {
		players = append(players, convertPlayer(rp))
		details = append(details, convertPlayerDetail(rp))
	}
	return players, details
}

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
	hasParsedPlayer := false
	for _, p := range rm.Players {
		if p.PurchaseLog != nil || len(p.GoldT) > 0 || len(p.XPT) > 0 {
			hasParsedPlayer = true
			break
		}
	}
	return hasParsedPlayer
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

func expandTimeseries(players []rawPlayer) []matchstore.TimeseriesRow {
	out := make([]matchstore.TimeseriesRow, 0, len(players)*60)
	for _, p := range players {
		// Use the "times" array as the authoritative time axis when available.
		// Fall back to the longest per-minute array if times is missing.
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

func deref64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

func deref32(p *int32) int32 {
	if p == nil {
		return 0
	}
	return *p
}

func deref16(p *int16) int16 {
	if p == nil {
		return 0
	}
	return *p
}

func derefBool(p *bool) bool {
	if p == nil {
		return false
	}
	return *p
}

func derefF32(p *float32) float32 {
	if p == nil {
		return 0
	}
	return *p
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func decToBool(v *int16) *bool {
	if v == nil {
		return nil
	}
	b := *v != 0
	return &b
}

func safeSlice(s []int32) []int32 {
	if len(s) == 0 {
		return nil
	}
	out := make([]int32, len(s))
	copy(out, s)
	return out
}

func safeIdx(s []int32, i int) *int32 {
	if i < len(s) {
		return &s[i]
	}
	return nil
}

func safeIdxSmall(s []int32, i int) *int16 {
	if i < len(s) {
		val := s[i]
		if val > math.MaxInt16 {
			val = math.MaxInt16
		}
		v := int16(val)
		return &v
	}
	return nil
}

type rawMatch struct {
	MatchID               int64           `json:"match_id"`
	MatchSeqNum           *int64          `json:"match_seq_num"`
	StartTime             int64           `json:"start_time"`
	Duration              int32           `json:"duration"`
	RadiantWin            *bool           `json:"radiant_win"`
	TowerStatusRadiant    *int16          `json:"tower_status_radiant"`
	TowerStatusDire       *int16          `json:"tower_status_dire"`
	BarracksStatusRadiant *int16          `json:"barracks_status_radiant"`
	BarracksStatusDire    *int16          `json:"barracks_status_dire"`
	RadiantScore          *int16          `json:"radiant_score"`
	DireScore             *int16          `json:"dire_score"`
	FirstBloodTime        *int32          `json:"first_blood_time"`
	LobbyType             *int16          `json:"lobby_type"`
	GameMode              *int16          `json:"game_mode"`
	Cluster               *int16          `json:"cluster"`
	Region                *int16          `json:"region"`
	Skill                 *int16          `json:"skill"`
	Engine                *int16          `json:"engine"`
	HumanPlayers          *int16          `json:"human_players"`
	Version               *int16          `json:"version"`
	Patch                 *int32          `json:"patch"`
	PositiveVotes         *int32          `json:"positive_votes"`
	NegativeVotes         *int32          `json:"negative_votes"`
	LeagueID              *int32          `json:"leagueid"`
	SeriesID              *int32          `json:"series_id"`
	SeriesType            *int16          `json:"series_type"`
	RadiantTeamID         *int64          `json:"radiant_team_id"`
	DireTeamID            *int64          `json:"dire_team_id"`
	RadiantCaptain        *int64          `json:"radiant_captain"`
	DireCaptain           *int64          `json:"dire_captain"`
	ReplaySalt            *int64          `json:"replay_salt"`
	ReplayURL             *string         `json:"replay_url"`
	Pauses                json.RawMessage `json:"pauses"`
	Cosmetics             json.RawMessage `json:"cosmetics"`

	Players      []rawPlayer      `json:"players"`
	PicksBans    []rawPickBan     `json:"picks_bans"`
	DraftTimings []rawDraftTiming `json:"draft_timings"`
	Objectives   []rawObjective   `json:"objectives"`
	Chat         []rawChat        `json:"chat"`
	Teamfights   []rawTeamfight   `json:"teamfights"`
	RadiantGoldAdv []int32        `json:"radiant_gold_adv"`
	RadiantXPAdv   []int32        `json:"radiant_xp_adv"`
}

type rawPlayer struct {
	PlayerSlot              int16           `json:"player_slot"`
	AccountID               *int64          `json:"account_id"`
	HeroID                  int16           `json:"hero_id"`
	HeroVariant             *int16          `json:"hero_variant"`
	IsRadiant               *bool           `json:"isRadiant"`
	Win                     *int16          `json:"win"`
	PatchID                 *int32          `json:"patch"`
	LobbyType               *int16          `json:"lobby_type"`
	GameMode                *int16          `json:"game_mode"`
	RankTier                *int16          `json:"rank_tier"`
	Kills                   int16           `json:"kills"`
	Deaths                  int16           `json:"deaths"`
	Assists                 int16           `json:"assists"`
	Level                   *int16          `json:"level"`
	NetWorth                *int32          `json:"net_worth"`
	Gold                    *int32          `json:"gold"`
	GoldSpent               *int32          `json:"gold_spent"`
	GoldPerMin              *int16          `json:"gold_per_min"`
	XPPerMin                *int16          `json:"xp_per_min"`
	LastHits                *int16          `json:"last_hits"`
	Denies                  *int16          `json:"denies"`
	HeroDamage              *int32          `json:"hero_damage"`
	TowerDamage             *int32          `json:"tower_damage"`
	HeroHealing             *int32          `json:"hero_healing"`
	Item0                   *int32          `json:"item_0"`
	Item1                   *int32          `json:"item_1"`
	Item2                   *int32          `json:"item_2"`
	Item3                   *int32          `json:"item_3"`
	Item4                   *int32          `json:"item_4"`
	Item5                   *int32          `json:"item_5"`
	ItemNeutral             *int32          `json:"item_neutral"`
	Backpack0               *int32          `json:"backpack_0"`
	Backpack1               *int32          `json:"backpack_1"`
	Backpack2               *int32          `json:"backpack_2"`
	Backpack3               *int32          `json:"backpack_3"`
	Lane                    *int16          `json:"lane"`
	LaneRole                *int16          `json:"lane_role"`
	IsRoaming               *bool           `json:"is_roaming"`
	PartyID                 *int32          `json:"party_id"`
	PartySize               *int16          `json:"party_size"`
	Stuns                   *float32        `json:"stuns"`
	ObsPlaced               *int16          `json:"obs_placed"`
	SenPlaced               *int16          `json:"sen_placed"`
	CreepsStacked           *int16          `json:"creeps_stacked"`
	CampsStacked            *int16          `json:"camps_stacked"`
	RunePickups             *int16          `json:"rune_pickups"`
	FirstbloodClaimed       *int16          `json:"firstblood_claimed"`
	TeamfightParticipation  *float32        `json:"teamfight_participation"`
	TowersKilled            *int16          `json:"towers_killed"`
	RoshansKilled           *int16          `json:"roshans_killed"`
	ObserversPlaced         *int16          `json:"observers_placed"`
	LeaverStatus            *int16          `json:"leaver_status"`
	GoldT                   []int32         `json:"gold_t"`
	XPT                     []int32         `json:"xp_t"`
	LHT                     []int32         `json:"lh_t"`
	DNT                     []int32         `json:"dn_t"`
	Times                   []int32         `json:"times"`
	ThrowGold               *int32          `json:"throw"`
	ComebackGold            *int32          `json:"comeback"`
	LossGold                *int32          `json:"loss"`
	WinGold                 *int32          `json:"win_gold"`

	Damage                  json.RawMessage `json:"damage"`
	DamageTaken             json.RawMessage `json:"damage_taken"`
	DamageInflictor         json.RawMessage `json:"damage_inflictor"`
	DamageInflictorReceived json.RawMessage `json:"damage_inflictor_received"`
	DamageTargets           json.RawMessage `json:"damage_targets"`
	HeroHits                json.RawMessage `json:"hero_hits"`
	MaxHeroHit              json.RawMessage `json:"max_hero_hit"`
	AbilityUses             json.RawMessage `json:"ability_uses"`
	AbilityTargets          json.RawMessage `json:"ability_targets"`
	AbilityUpgradesArr      json.RawMessage `json:"ability_upgrades_arr"`
	ItemUses                json.RawMessage `json:"item_uses"`
	GoldReasons             json.RawMessage `json:"gold_reasons"`
	XPReasons               json.RawMessage `json:"xp_reasons"`
	Killed                  json.RawMessage `json:"killed"`
	KilledBy                json.RawMessage `json:"killed_by"`
	KillStreaks             json.RawMessage `json:"kill_streaks"`
	MultiKills              json.RawMessage `json:"multi_kills"`
	LifeState               json.RawMessage `json:"life_state"`
	LanePos                 json.RawMessage `json:"lane_pos"`
	Obs                     json.RawMessage `json:"obs"`
	Sen                     json.RawMessage `json:"sen"`
	Actions                 json.RawMessage `json:"actions"`
	Pings                   json.RawMessage `json:"pings"`
	Runes                   json.RawMessage `json:"runes"`
	Purchase                json.RawMessage `json:"purchase"`
	ObsLog                  json.RawMessage `json:"obs_log"`
	SenLog                  json.RawMessage `json:"sen_log"`
	ObsLeftLog              json.RawMessage `json:"obs_left_log"`
	SenLeftLog              json.RawMessage `json:"sen_left_log"`
	PurchaseLog             json.RawMessage `json:"purchase_log"`
	KillsLog                json.RawMessage `json:"kills_log"`
	BuybackLog              json.RawMessage `json:"buyback_log"`
	RunesLog                json.RawMessage `json:"runes_log"`
	ConnectionLog           json.RawMessage `json:"connection_log"`
	PermanentBuffs          json.RawMessage `json:"permanent_buffs"`
	NeutralTokensLog        json.RawMessage `json:"neutral_tokens_log"`
	NeutralItemHistory      json.RawMessage `json:"neutral_item_history"`
	AdditionalUnits         json.RawMessage `json:"additional_units"`
	Cosmetics               json.RawMessage `json:"cosmetics"`
	Benchmarks              json.RawMessage `json:"benchmarks"`
	AllWordCounts           json.RawMessage `json:"all_word_counts"`
	MyWordCounts            json.RawMessage `json:"my_word_counts"`
}

type rawPickBan struct {
	Order  int16 `json:"order"`
	IsPick bool  `json:"is_pick"`
	HeroID int16 `json:"hero_id"`
	Team   int16 `json:"team"`
}

type rawDraftTiming struct {
	Order          int16  `json:"order"`
	Pick           bool   `json:"pick"`
	ActiveTeam     *int16 `json:"active_team"`
	HeroID         *int16 `json:"hero_id"`
	PlayerSlot     *int16 `json:"player_slot"`
	ExtraTime      *int32 `json:"extra_time"`
	TotalTimeTaken *int32 `json:"total_time_taken"`
}

type rawObjective struct {
	Time       int32           `json:"time"`
	Type       string          `json:"type"`
	Slot       *int16          `json:"slot"`
	PlayerSlot *int16          `json:"player_slot"`
	Team       *int16          `json:"team"`
	Key        json.RawMessage `json:"key"`
	Value      *int32          `json:"value"`
	Unit       *string         `json:"unit"`
}

type rawChat struct {
	Time       int32   `json:"time"`
	Type       *string `json:"type"`
	PlayerSlot *int16  `json:"player_slot"`
	Unit       *string `json:"unit"`
	Key        *string `json:"key"`
}

type rawTeamfight struct {
	End       int32           `json:"end"`
	LastDeath *int32          `json:"last_death"`
	Deaths    *int16          `json:"deaths"`
	Players   json.RawMessage `json:"players"`
}