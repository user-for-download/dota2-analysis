// Package domain provides pure domain types for Dota 2 match entities.
//
// These types decouple match data from any specific storage or ingestion
// implementation. No imports from go-ingestion, matchstore, or database
// packages are allowed — only encoding/json for Raw fields.
package domain

import "encoding/json"

// MatchIdentity holds primary key, timing, and outcome.
type MatchIdentity struct {
	MatchID     MatchID
	MatchSeqNum int64
	StartTime   int64
	Duration    int32
	RadiantWin  *bool // nil = abandoned/incomplete match (per OpenDota API)
}

// MatchBuildings tracks tower/barracks status.
type MatchBuildings struct {
	TowerStatusRadiant    int16
	TowerStatusDire       int16
	BarracksStatusRadiant int16
	BarracksStatusDire    int16
	RadiantScore          int16
	DireScore             int16
}

// MatchLobby describes lobby configuration.
type MatchLobby struct {
	FirstBloodTime int32
	LobbyType      int16
	GameMode       int16
	Cluster        int16
	Region         int16
	Skill          int16
	Engine         int16
	HumanPlayers   int16
	Version        int16
}

// MatchContext holds patch, voting, league, series metadata.
type MatchContext struct {
	PatchID       PatchID
	PositiveVotes int32
	NegativeVotes int32
	LeagueID      int32
	SeriesID      int32
	SeriesType    int16
}

// MatchTeams identifies participating teams and captains.
type MatchTeams struct {
	RadiantTeamID  TeamID
	DireTeamID     TeamID
	RadiantCaptain AccountID
	DireCaptain    AccountID
}

// MatchReplay holds replay metadata.
type MatchReplay struct {
	ReplaySalt int64
	ReplayURL  string
	Pauses     json.RawMessage
}

// Player holds per-player match stats.
type Player struct {
	PlayerSlot              int16
	AccountID               AccountID
	HeroID                  HeroID
	HeroVariant             int16
	IsRadiant               bool
	Win                     bool
	PatchID                 PatchID
	LobbyType               int16
	GameMode                int16
	RankTier                int16
	Kills                   int16
	Deaths                  int16
	Assists                 int16
	Level                   int16
	NetWorth                int32
	Gold                    int32
	GoldSpent               int32
	GoldPerMin              int16
	XPPerMin                int16
	LastHits                int16
	Denies                  int16
	HeroDamage              int32
	TowerDamage             int32
	HeroHealing             int32
	Item0                   int32
	Item1                   int32
	Item2                   int32
	Item3                   int32
	Item4                   int32
	Item5                   int32
	ItemNeutral             int32
	Backpack0               int32
	Backpack1               int32
	Backpack2               int32
	Backpack3               int32
	Lane                    int16
	LaneRole                int16
	IsRoaming               bool
	PartyID                 int32
	PartySize               int16
	Stuns                   float32
	ObsPlaced               int16
	SenPlaced               int16
	CreepsStacked           int16
	CampsStacked            int16
	RunePickups             int16
	FirstbloodClaimed       bool
	TeamfightParticipation  float32
	TowersKilled            int16
	RoshansKilled           int16
	ObserversPlaced         int16
	LeaverStatus            int16
	GoldT                   []int32
	XPT                     []int32
	LHT                     []int32
	DNT                     []int32
	Times                   []int32
	ThrowGold               int32
	ComebackGold            int32
	LossGold                int32
	WinGold                 int32
}

// PlayerDetail holds per-player JSON blob details.
type PlayerDetail struct {
	PlayerSlot                int16
	Damage                    json.RawMessage
	DamageTaken               json.RawMessage
	DamageInflictor           json.RawMessage
	DamageInflictorReceived   json.RawMessage
	DamageTargets             json.RawMessage
	HeroHits                  json.RawMessage
	MaxHeroHit                json.RawMessage
	AbilityUses               json.RawMessage
	AbilityTargets            json.RawMessage
	AbilityUpgradesArr        json.RawMessage
	ItemUses                  json.RawMessage
	GoldReasons               json.RawMessage
	XPReasons                 json.RawMessage
	Killed                    json.RawMessage
	KilledBy                  json.RawMessage
	KillStreaks               json.RawMessage
	MultiKills                json.RawMessage
	LifeState                 json.RawMessage
	LanePos                   json.RawMessage
	Obs                       json.RawMessage
	Sen                       json.RawMessage
	Actions                   json.RawMessage
	Pings                     json.RawMessage
	Runes                     json.RawMessage
	Purchase                  json.RawMessage
	ObsLog                    json.RawMessage
	SenLog                    json.RawMessage
	ObsLeftLog                json.RawMessage
	SenLeftLog                json.RawMessage
	PurchaseLog               json.RawMessage
	KillsLog                  json.RawMessage
	BuybackLog                json.RawMessage
	RunesLog                  json.RawMessage
	ConnectionLog             json.RawMessage
	PermanentBuffs            json.RawMessage
	NeutralTokensLog          json.RawMessage
	NeutralItemHistory        json.RawMessage
	AdditionalUnits           json.RawMessage
	Cosmetics                 json.RawMessage
	Benchmarks                json.RawMessage
	AllWordCounts             json.RawMessage
	MyWordCounts              json.RawMessage
}

// PickBan represents a single pick or ban.
type PickBan struct {
	Order  int16
	IsPick bool
	HeroID HeroID
	Team   int16
}

// DraftTiming records timing details for a pick.
type DraftTiming struct {
	Order          int16
	Pick           bool
	ActiveTeam     int16
	HeroID         HeroID
	PlayerSlot     int16
	ExtraTime      int32
	TotalTimeTaken int32
}

// Objective represents an in-game objective.
type Objective struct {
	Time       int32
	Type       string
	Slot       int16
	PlayerSlot int16
	Team       int16
	Key        string
	Value      int32
	Unit       string
	Raw        json.RawMessage
}

// Chat represents a chat message.
type Chat struct {
	Time       int32
	Type       string
	PlayerSlot int16
	Unit       string
	Key        string
}

// Teamfight summarizes a teamfight.
type Teamfight struct {
	EndTime   int32
	LastDeath int32
	Deaths    int16
	Players   json.RawMessage
}

// Advantages holds radiant advantage over time.
type Advantages struct {
	RadiantGoldAdv []int32
	RadiantXPAdv   []int32
}

// TimeseriesRow holds per-player per-minute stats.
type TimeseriesRow struct {
	PlayerSlot int16
	Minute     int16
	HeroID     HeroID
	AccountID  AccountID
	PatchID    PatchID
	Gold       int32
	XP         int32
	LH         int16
	DN         int16
}

// Match is the root domain object for a parsed Dota 2 match.
type Match struct {
	MatchIdentity
	MatchBuildings
	MatchLobby
	MatchContext
	MatchTeams
	MatchReplay
	IsParsed bool

	Players      []Player
	Details      []PlayerDetail
	PicksBans    []PickBan
	DraftTimings []DraftTiming
	Objectives   []Objective
	Chat         []Chat
	Teamfights   []Teamfight
	Advantages   *Advantages
	Cosmetics    json.RawMessage
	Timeseries   []TimeseriesRow

	// Raw JSON payload (e.g. stored as-is for replay).
	Raw json.RawMessage
}
