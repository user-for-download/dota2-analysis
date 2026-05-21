// Package refdatastore defines the storage port for reference data
// (heroes, items, abilities, patches, teams, …) ingested from external
// sources such as the dotaconstants project.
//
// The package is intentionally storage-agnostic: it owns the data
// transfer types (HeroRef, ItemRef, …) and the writer interface, but
// no SQL, no JSON, and no HTTP. Concrete adapters live in subpackages
// (refdatapg for Postgres). Domain packages such as `enrich` consume
// RefDataWriter; they do not import any adapter.
package refdatastore

import (
	"context"
	"encoding/json"
	"time"
)

type RefDataWriter interface {
	UpsertHeroes(ctx context.Context, heroes []HeroRef) error
	UpsertHeroStats(ctx context.Context, hs []HeroStatRef) error
	UpsertHeroAbilities(ctx context.Context, ha []HeroAbilityRef) error
	UpsertHeroTalents(ctx context.Context, ht []HeroTalentRef) error
	UpsertHeroFacets(ctx context.Context, hf []HeroFacetRef) error
	UpsertAbilities(ctx context.Context, a []AbilityRef) error
	UpsertAbilityIDs(ctx context.Context, ai []AbilityIDRef) error
	UpsertItems(ctx context.Context, items []ItemRef) error
	UpsertItemIDs(ctx context.Context, ii []ItemIDRef) error
	UpsertPatches(ctx context.Context, p []PatchRef) error
	UpsertTeams(ctx context.Context, teams []TeamRef) error
	UpsertLeagues(ctx context.Context, leagues []LeagueRef) error
	UpsertNotablePlayers(ctx context.Context, np []NotablePlayerRef) error
	UpsertProPlayers(ctx context.Context, pp []ProPlayerRef) error
	UpsertGameModes(ctx context.Context, gm []GameModeRef) error
	UpsertLobbyTypes(ctx context.Context, lt []LobbyTypeRef) error
	UpsertRegions(ctx context.Context, reg []RegionRef) error
}

type HeroesWriter interface {
	UpsertHeroes(ctx context.Context, heroes []HeroRef) error
}

type HeroAbilitiesWriter interface {
	UpsertHeroAbilities(ctx context.Context, ha []HeroAbilityRef) error
	UpsertHeroTalents(ctx context.Context, ht []HeroTalentRef) error
	UpsertHeroFacets(ctx context.Context, hf []HeroFacetRef) error
}

type HeroStatsWriter interface {
	UpsertHeroStats(ctx context.Context, hs []HeroStatRef) error
}

type ItemsWriter interface {
	UpsertItems(ctx context.Context, items []ItemRef) error
}

type ItemIDsWriter interface {
	UpsertItemIDs(ctx context.Context, ii []ItemIDRef) error
}

type LeaguesWriter interface {
	UpsertLeagues(ctx context.Context, leagues []LeagueRef) error
}

type TeamsWriter interface {
	UpsertTeams(ctx context.Context, teams []TeamRef) error
}

type PatchesWriter interface {
	UpsertPatches(ctx context.Context, p []PatchRef) error
}

type GameModesWriter interface {
	UpsertGameModes(ctx context.Context, gm []GameModeRef) error
}

type LobbyTypesWriter interface {
	UpsertLobbyTypes(ctx context.Context, lt []LobbyTypeRef) error
}

type RegionsWriter interface {
	UpsertRegions(ctx context.Context, reg []RegionRef) error
}

type AbilitiesWriter interface {
	UpsertAbilities(ctx context.Context, a []AbilityRef) error
}

type AbilityIDsWriter interface {
	UpsertAbilityIDs(ctx context.Context, ai []AbilityIDRef) error
}

type NotablePlayersWriter interface {
	UpsertNotablePlayers(ctx context.Context, np []NotablePlayerRef) error
}

type ProPlayersWriter interface {
	UpsertProPlayers(ctx context.Context, pp []ProPlayerRef) error
}

type HeroRef struct {
	ID             int
	Name           string
	LocalizedName  string
	PrimaryAttr    string
	AttackType     string
	Roles          []string
	Legs           int
}

type HeroStatRef struct {
	HeroID    int
	Winrate   float64
	Pickrate  float64
	BannedPct float64
}

type HeroAbilityRef struct {
	HeroName string
	Slot     int
	Ability  string
}

type HeroTalentRef struct {
	HeroName string
	Ability  string
	Level    int
}

type HeroFacetRef struct {
	HeroName    string
	Slot        int
	Name        string
	Title       string
	Description string
	Icon        string
	Color       string
	GradientID  int
	Deprecated  bool
}

type AbilityRef struct {
	Name        string
	Localized   string
	Behavior    json.RawMessage
	TargetTeam  string
	Description string
	Img         string
	ManaCost    string
	Cooldown    string
	Attrib      json.RawMessage
	IsTalent    bool
}

type AbilityIDRef struct {
	ID   int
	Name string
}

type ItemRef struct {
	ID       int
	Key      string
	DName    string
	Cost     int
	Qual     string
	Behavior string
	Lore     string
	Img      string
	Created  bool
	Cooldown string
	ManaCost string
}

type ItemIDRef struct {
	ID  int
	Key string
}

type PatchRef struct {
	ID        int
	Name      string
	ReleaseAt time.Time
}

type LeagueRef struct {
	ID      int
	Name   string
	Tier   string
	Region string
	StartAt time.Time
	EndAt  time.Time
}

type TeamRef struct {
	ID      int
	Name   string
	Tag    string
	LogoURL string
	Country string
}

type NotablePlayerRef struct {
	AccountID int64
	Name      string
	TeamID    int64
	Country   string
}

type ProPlayerRef struct {
	AccountID int64
	Name      string
	TeamID    int64
	Country   string
	IsPro     bool
}

type GameModeRef struct {
	ID       int
	Name     string
	Balanced bool
}

type LobbyTypeRef struct {
	ID       int
	Name     string
	Balanced bool
}

type RegionRef struct {
	ID   int
	Name string
}