package refdatapg

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/enrich"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/storage/refdatastore"
)

type Store struct {
	db  *pgxpool.Pool
	log *slog.Logger
}

func NewStore(db *pgxpool.Pool, log *slog.Logger) *Store {
	if log == nil {
		log = slog.Default()
	}
	return &Store{db: db, log: log.With("component", "refdatapg")}
}

var (
	_ refdatastore.RefDataWriter   = (*Store)(nil)
	_ enrich.HeroesWriter          = (*Store)(nil)
	_ enrich.HeroAbilitiesWriter   = (*Store)(nil)
	_ enrich.HeroStatsWriter       = (*Store)(nil)
	_ enrich.ItemsWriter           = (*Store)(nil)
	_ enrich.ItemIDsWriter         = (*Store)(nil)
	_ enrich.LeaguesWriter         = (*Store)(nil)
	_ enrich.TeamsWriter           = (*Store)(nil)
	_ enrich.PatchesWriter         = (*Store)(nil)
	_ enrich.GameModesWriter       = (*Store)(nil)
	_ enrich.LobbyTypesWriter      = (*Store)(nil)
	_ enrich.RegionsWriter         = (*Store)(nil)
	_ enrich.AbilitiesWriter       = (*Store)(nil)
	_ enrich.AbilityIDsWriter      = (*Store)(nil)
	_ enrich.NotablePlayersWriter  = (*Store)(nil)
	_ enrich.ProPlayersWriter      = (*Store)(nil)
)