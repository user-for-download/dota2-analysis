package profiles

import (
	"context"
	"time"

	"github.com/user-for-download/go-dota2-analysis/internal/domain"
)

// HeroPair represents synergy or counter data between two heroes.
type HeroPair struct {
	HeroID   domain.HeroID
	HeroName string
	Games    int
	Wins     int
	WRShrunk float64
}

// TeamHero represents a team's performance with a hero.
type TeamHero struct {
	HeroID   domain.HeroID
	HeroName string
	Games    int
	Wins     int
	WRShrunk float64
}

// PlayerHero represents a player's performance with a hero.
type PlayerHero struct {
	HeroID     domain.HeroID
	HeroName   string
	Games      int
	Wins       int
	WRShrunk   float64
	LastPlayed time.Time
}

// PlayerTeam represents a player's team affiliation.
type PlayerTeam struct {
	TeamID      int64
	Games       int
	Wins        int
	LastPlayed  time.Time
	LastPatchID int
}

// H2HRecord represents head-to-head stats between two teams.
type H2HRecord struct {
	Games     int
	TeamAWins int
	TeamBWins int
}

// FeaturizerStatus represents the featurizer's health status.
type FeaturizerStatus struct {
	LastSuccessful time.Time
}

// TeamHeroStats is the games+winrate pair used by feature sources.
type TeamHeroStats struct {
	Games    int
	WRShrunk float64
}

// Repository provides read access to analytics data.
// All SQL is encapsulated here — callers know nothing about Postgres.
type Repository interface {
	// Hero queries
	HeroSynergies(ctx context.Context, heroID domain.HeroID, minGames, limit int) ([]HeroPair, error)
	HeroCounters(ctx context.Context, heroID domain.HeroID, minGames, limit int) ([]HeroPair, error)

	// Team queries
	TeamHeroes(ctx context.Context, teamID domain.TeamID, minGames, limit int) ([]TeamHero, error)
	TeamH2H(ctx context.Context, teamA, teamB domain.TeamID) (H2HRecord, error)

	// Player queries
	PlayerHeroes(ctx context.Context, accountID domain.AccountID, minGames, limit int) ([]PlayerHero, error)
	PlayerTeams(ctx context.Context, accountID domain.AccountID, limit int) ([]PlayerTeam, error)

	// Featurizer health
	FeaturizerStatus(ctx context.Context) (FeaturizerStatus, error)

	// Feature-source batch lookups (one round-trip per call).
	TeamHeroStatsBatch(ctx context.Context, teamID domain.TeamID, heroes []domain.HeroID) (map[domain.HeroID]TeamHeroStats, error)
	SynergyAvgBatch(ctx context.Context, allies []domain.HeroID, candidates []domain.HeroID) (map[domain.HeroID]float64, error)
	CounterAvgBatch(ctx context.Context, candidates []domain.HeroID, enemies []domain.HeroID) (map[domain.HeroID]float64, error)
	RosterComfortAvgBatch(ctx context.Context, roster []domain.AccountID, heroes []domain.HeroID) (map[domain.HeroID]float64, error)
	StarThreatBatch(ctx context.Context, themTeamID domain.TeamID, heroes []domain.HeroID, minGames int) (map[domain.HeroID]float64, error)
}
