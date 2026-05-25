package pgclient

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/storage/lookupstore"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/storage/lookupstore/lookuppg"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/storage/matchstore"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/storage/matchstore/matchpg"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/storage/partitionstore"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/storage/partitionstore/partitionpg"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/storage/refdatastore"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/storage/refdatastore/refdatapg"
)

type Stores struct {
	Matches    matchstore.MatchStore
	Lookups    lookupstore.LookupReader
	Partitions partitionstore.PartitionAdmin
	RefData    refdatastore.RefDataWriter
}

func NewStores(db *pgxpool.Pool, log *slog.Logger) Stores {
	if log == nil {
		log = slog.Default()
	}
	return Stores{
		Matches:    matchpg.NewStore(db, log),
		Lookups:    lookuppg.NewStore(db, log),
		Partitions: partitionpg.NewAdmin(db, log),
		RefData:    refdatapg.NewStore(db, log),
	}
}