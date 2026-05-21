package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/user-for-download/go-dota2/internal/bootstrap"
	"github.com/user-for-download/go-dota2/internal/config"
)

// Lua script for atomic XAdd + XDel: adds message to target stream and deletes from DLQ.
// ARGV layout: [1]=maxLen, [2]=DLQ message ID, [3..]=field-value pairs for XADD.
// Returns XDEL count (1 on success, 0 if message already gone).
var luaReplayAtomic = `
local added = redis.call('XADD', KEYS[1], 'MAXLEN', '~', ARGV[1], '*', unpack(ARGV, 3))
local deleted = redis.call('XDEL', KEYS[2], ARGV[2])
return deleted
`

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

	// Initialize telemetry for observability consistency with other binaries.
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

	dlqStreams := []string{}
	if *streamType == "all" || *streamType == "fetch" {
		dlqStreams = append(dlqStreams, cfg.Queue.FetchDLQStream)
	}
	if *streamType == "all" || *streamType == "parse" {
		dlqStreams = append(dlqStreams, cfg.Queue.ParseDLQStream)
	}
	if len(dlqStreams) == 0 {
		log.Error("no DLQ streams configured")
		os.Exit(1)
	}

	switch *action {
	case "list":
		err = cmdList(ctx, rdb, dlqStreams, *limit, log)
	case "replay":
		err = cmdReplay(ctx, rdb, dlqStreams, cfg.Queue, *limit, *dryRun, log)
	case "purge":
		err = cmdPurge(ctx, rdb, dlqStreams, *limit, *dryRun, log)
	}
	must(log, *action, err)
}

func cmdList(ctx context.Context, rdb *redis.Client, streams []string, limit int, log *slog.Logger) error {
	for _, s := range streams {
		msgs, err := rdb.XRevRangeN(ctx, s, "+", "-", int64(limit)).Result()
		if err != nil {
			return fmt.Errorf("xrevrange %s: %w", s, err)
		}
		log.Info("stream", "name", s, "count", len(msgs))
		for _, m := range msgs {
			retries := "0"
			if r, ok := m.Values["r"]; ok {
				retries = fmt.Sprintf("%v", r)
			}
			reason := ""
			if r, ok := m.Values["reason"]; ok {
				reason = fmt.Sprintf("%v", r)
			}
			payloadLen := 0
			if p, ok := m.Values["p"]; ok {
				switch v := p.(type) {
				case string:
					payloadLen = len(v)
				case []byte:
					payloadLen = len(v)
				}
			}
			log.Info("task", "stream", s, "id", m.ID, "retries", retries, "reason", reason, "payload_bytes", payloadLen)
		}
	}
	return nil
}

// extractMatchID parses the payload JSON to extract match_id safely.
// Returns empty string if match_id cannot be found.
func extractMatchID(payload any) string {
	var pStr string
	switch v := payload.(type) {
	case string:
		pStr = v
	case []byte:
		pStr = string(v)
	default:
		return ""
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(pStr), &raw); err != nil {
		return ""
	}
	if mid, ok := raw["match_id"]; ok {
		// Strip quotes if present (JSON string) or parse as number
		var n int64
		if err := json.Unmarshal(mid, &n); err == nil {
			return fmt.Sprintf("%d", n)
		}
		var s string
		if err := json.Unmarshal(mid, &s); err == nil {
			return s
		}
	}
	return ""
}

// replayValues builds the XAdd values map, preserving OTel trace context
// from the original DLQ message.
func replayValues(dlqMsg redis.XMessage, payload any) map[string]any {
	vals := map[string]any{"p": payload, "r": "0"}
	// Preserve W3C trace context for end-to-end traceability.
	if tp, ok := dlqMsg.Values["_otel_traceparent"]; ok {
		vals["_otel_traceparent"] = tp
	}
	if ts, ok := dlqMsg.Values["_otel_tracestate"]; ok {
		vals["_otel_tracestate"] = ts
	}
	return vals
}

func cmdReplay(ctx context.Context, rdb *redis.Client, dlqStreams []string, qCfg config.QueueConfig, limit int, dryRun bool, log *slog.Logger) error {
	mapping := map[string]string{
		qCfg.FetchDLQStream: qCfg.FetchStream,
		qCfg.ParseDLQStream: qCfg.ParseStream,
	}

	replayCmd := redis.NewScript(luaReplayAtomic)

	for _, dlq := range dlqStreams {
		target := mapping[dlq]
		if target == "" {
			continue
		}

		// Scope guard key per stream type to avoid cross-DLQ interference.
		streamLabel := "unknown"
		if target == qCfg.FetchStream {
			streamLabel = "fetch"
		} else if target == qCfg.ParseStream {
			streamLabel = "parse"
		}
		guardKey := "dota2:dlq:guard:" + streamLabel
		guardTTL := 7 * 24 * time.Hour

		msgs, err := rdb.XRevRangeN(ctx, dlq, "+", "-", int64(limit)).Result()
		if err != nil {
			return fmt.Errorf("xrevrange %s: %w", dlq, err)
		}

		if dryRun {
			log.Info("dry-run: would replay", "dlq", dlq, "target", target, "count", len(msgs))
			continue
		}

		replayed := 0
		skipped := 0
		failed := 0
		for _, m := range msgs {
			payload := m.Values["p"]
			matchID := extractMatchID(payload)

			if matchID != "" {
				added, err := rdb.SetNX(ctx, guardKey+":"+matchID, "1", guardTTL).Result()
				if err != nil {
					log.Warn("guard setnx failed", "err", err)
				}
				if !added {
					skipped++
					log.Info("skipping duplicate match_id in DLQ replay", "match_id", matchID, "dlq_id", m.ID, "stream", streamLabel)
					// Do NOT delete the DLQ message — it may contain failure context
					// worth preserving for manual inspection.
					continue
				}
			}

			// Atomic XAdd + XDel via Lua script to prevent duplicates on partial failure.
			// ARGV layout: [1]=maxLen, [2]=DLQ msg ID, [3..]=field-value pairs.
			keys := []string{target, dlq}
			args := []any{qCfg.MaxLen, m.ID}
			vals := replayValues(m, payload)
			// Flatten vals into args as alternating key-value pairs.
			for k, v := range vals {
				args = append(args, k, v)
			}

			result, err := replayCmd.Run(ctx, rdb, keys, args...).Int64()
			if err != nil {
				log.Error("atomic replay failed", "id", m.ID, "err", err)
				failed++
				continue
			}
			if result > 0 {
				replayed++
			} else {
				// XAdd succeeded but XDel failed — message stays in DLQ for safety.
				log.Warn("replay XAdd succeeded but XDel failed, message retained in DLQ", "id", m.ID)
				replayed++
			}
		}
		log.Info("replay done", "dlq", dlq, "replayed", replayed, "skipped", skipped, "failed", failed)
	}
	return nil
}

func cmdPurge(ctx context.Context, rdb *redis.Client, streams []string, limit int, dryRun bool, log *slog.Logger) error {
	for _, s := range streams {
		msgs, err := rdb.XRevRangeN(ctx, s, "+", "-", int64(limit)).Result()
		if err != nil {
			return fmt.Errorf("xrevrange %s: %w", s, err)
		}
		log.Info("purging", "stream", s, "count", len(msgs))
		if dryRun {
			log.Info("dry-run enabled, skipping purge")
			continue
		}
		if len(msgs) == 0 {
			continue
		}
		ids := make([]string, len(msgs))
		for i, m := range msgs {
			ids[i] = m.ID
		}
		if _, err := rdb.XDel(ctx, s, ids...).Result(); err != nil {
			return fmt.Errorf("xdel %s: %w", s, err)
		}
		log.Info("purged", "stream", s, "deleted", len(ids))
	}
	return nil
}

func must(log *slog.Logger, what string, err error) {
	if err != nil {
		log.Error(what, "err", err)
		os.Exit(1)
	}
}
