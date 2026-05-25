package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/config"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/storage/partitionstore"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/storage/pgclient"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/worker"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/worker/ingester"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/worker/parser"
)

type ParserDeps struct {
	Worker         *parser.Parser
	PartitionAdmin partitionstore.PartitionAdmin
}

func BuildParser(
	ctx context.Context,
	cfg *config.Config,
	log *slog.Logger,
	core *Deps,
	db *pgxpool.Pool,
) (*ParserDeps, error) {
	store, err := PayloadStore(core.Redis.Master(), cfg.Payload)
	if err != nil {
		return nil, fmt.Errorf("parser: payload store: %w", err)
	}

	parseQ, err := ParseQueue(core.Redis.Master(), cfg.Queue, log)
	if err != nil {
		return nil, fmt.Errorf("parser: parse queue: %w", err)
	}

	repo := MatchWriter(db, log)
	stores := pgclient.NewStores(db, log)
	partitionAdmin := stores.Partitions

	dedupSeen, err := DedupSeen(core.Redis.Master(), cfg.Dedup)
	if err != nil {
		log.Warn("parser: dedup init failed, proceeding without dedup", "err", err)
	}

	baseIng, err := ingester.New(repo, core.Metrics, ingester.Config{
		Logger: log,
		Dedup:  dedupSeen,
	})
	if err != nil {
		return nil, fmt.Errorf("parser: ingester: %w", err)
	}

	cb := worker.NewCircuitBreaker(10, 30*time.Second)
	ing := ingester.NewResilient(baseIng, cb, log)

	w, err := parser.New(parseQ, store, ing, core.Metrics, parser.Config{
		Batch: cfg.Parser.Batch,
		Block: cfg.Parser.Block,
		Logger: log,
	})
	if err != nil {
		return nil, fmt.Errorf("parser: init: %w", err)
	}

	return &ParserDeps{Worker: w, PartitionAdmin: partitionAdmin}, nil
}