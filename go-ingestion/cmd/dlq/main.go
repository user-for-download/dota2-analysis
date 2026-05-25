// Command dlq manages dead-letter queue streams in Redis.
//
// Actions:
//
//	list    — view recent DLQ messages
//	replay  — atomically re-queue messages back to their original stream
//	purge   — bulk-delete messages from the DLQ
//
// Usage:
//
//	go run ./cmd/dlq --action list --stream fetch --limit 20
//	go run ./cmd/dlq --action replay --stream parse --limit 50 --dry-run
//	go run ./cmd/dlq --action purge --stream all --limit 100
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/bootstrap"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/config"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/dlq"
)

func main() {
	log := bootstrap.NewLogger(slog.NewJSONHandler(os.Stdout, nil))

	action := flag.String("action", "list", "Action: list, replay, purge")
	streamType := flag.String("stream", "all", "DLQ stream: fetch, parse, all")
	limit := flag.Int("limit", 10, "Max tasks to process")
	dryRun := flag.Bool("dry-run", false, "Simulate replay/purge without modifying data")
	flag.Parse()

	switch *action {
	case "list", "replay", "purge":
	default:
		log.Error("invalid action", "valid", "list,replay,purge")
		os.Exit(1)
	}
	switch *streamType {
	case "fetch", "parse", "all":
	default:
		log.Error("invalid stream type", "valid", "fetch,parse,all")
		os.Exit(1)
	}
	if *limit <= 0 {
		log.Error("limit must be > 0")
		os.Exit(1)
	}

	cfg, err := config.Load("")
	must(log, "config", err)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdownTelemetry, err := bootstrap.InitTelemetry(ctx, "go-dota2-dlq", cfg.Telemetry.Endpoint, cfg.Telemetry.SampleRate)
	if err != nil {
		log.Error("init telemetry", "err", err)
	} else if shutdownTelemetry != nil {
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = shutdownTelemetry(shutdownCtx)
		}()
	}

	redisClient, err := bootstrap.RedisClient(cfg.Redis, log)
	must(log, "redis", err)
	defer redisClient.Close()

	rdb := redisClient.Master()

	dlqStreams := buildStreamList(*streamType, cfg.Queue)
	if len(dlqStreams) == 0 {
		log.Error("no DLQ streams configured")
		os.Exit(1)
	}

	switch *action {
	case "list":
		err = dlq.List(ctx, rdb, dlqStreams, *limit, log)
	case "replay":
		err = dlq.Replay(ctx, rdb, dlqStreams, cfg.Queue, *limit, *dryRun, log)
	case "purge":
		err = dlq.Purge(ctx, rdb, dlqStreams, *limit, *dryRun, log)
	}
	must(log, *action, err)
}

func buildStreamList(streamType string, qCfg config.QueueConfig) []string {
	var streams []string
	if streamType == "all" || streamType == "fetch" {
		streams = append(streams, qCfg.FetchDLQStream)
	}
	if streamType == "all" || streamType == "parse" {
		streams = append(streams, qCfg.ParseDLQStream)
	}
	return streams
}

func must(log *slog.Logger, what string, err error) {
	if err != nil {
		log.Error(what, "err", err)
		os.Exit(1)
	}
}
