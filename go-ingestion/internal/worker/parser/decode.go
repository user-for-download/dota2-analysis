package parser

import (
	"encoding/json"
	"math"

	"github.com/user-for-download/go-dota2/internal/storage/matchstore"
)

// Match is an alias for the matchstore type.
type Match = matchstore.Match

// ─── Raw JSON types ────────────────────────────────────────

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

// ─── Deref Helpers ─────────────────────────────────────────

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
