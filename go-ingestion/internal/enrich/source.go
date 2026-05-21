package enrich

import (
	"context"
	"log/slog"

	"github.com/user-for-download/go-dota2/internal/enrich/httpclient"
	initmarker "github.com/user-for-download/go-dota2/internal/enrich/initmarker"
	"github.com/user-for-download/go-dota2/internal/storage/refdatastore"
)

type HTTPClient = httpclient.HTTPClient
type BootstrapMarker = initmarker.BootstrapMarker

type RunSource interface {
	Name() string
	Critical() bool
	DependsOn() []string
	Run(ctx context.Context, deps Deps) error
}

type Deps struct {
	HTTP   HTTPClient
	Writer RefDataWriter
	Logger *slog.Logger
}

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

type HeroRef = refdatastore.HeroRef
type HeroStatRef = refdatastore.HeroStatRef
type HeroAbilityRef = refdatastore.HeroAbilityRef
type HeroTalentRef = refdatastore.HeroTalentRef
type HeroFacetRef = refdatastore.HeroFacetRef
type AbilityRef = refdatastore.AbilityRef
type AbilityIDRef = refdatastore.AbilityIDRef
type ItemRef = refdatastore.ItemRef
type ItemIDRef = refdatastore.ItemIDRef
type PatchRef = refdatastore.PatchRef
type TeamRef = refdatastore.TeamRef
type LeagueRef = refdatastore.LeagueRef
type NotablePlayerRef = refdatastore.NotablePlayerRef
type ProPlayerRef = refdatastore.ProPlayerRef
type GameModeRef = refdatastore.GameModeRef
type LobbyTypeRef = refdatastore.LobbyTypeRef
type RegionRef = refdatastore.RegionRef