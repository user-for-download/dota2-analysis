// Package dlq provides the dead-letter queue operations: list, replay, and purge.
//
// DLQ messages are stored in Redis streams. Replay atomically copies a message
// back to the original stream while deleting it from the DLQ (via Lua script).
// Purge bulk-deletes messages from the DLQ.
package dlq

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/user-for-download/go-dota2/internal/config"
)

// luaReplayAtomic is a Lua script for atomic XAdd + XDel.
// ARGV layout: [1]=maxLen, [2]=DLQ message ID, [3..]=field-value pairs for XADD.
// Returns XDEL count (1 on success, 0 if message already gone).
var luaReplayAtomic = `
local added = redis.call('XADD', KEYS[1], 'MAXLEN', '~', ARGV[1], '*', unpack(ARGV, 3))
local deleted = redis.call('XDEL', KEYS[2], ARGV[2])
return deleted
`

// List prints the most recent messages in the given DLQ streams.
func List(ctx context.Context, rdb *redis.Client, streams []string, limit int, log *slog.Logger) error {
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

// Replay atomically copies DLQ messages back to their original streams and
// removes them from the DLQ. Uses a Lua script for atomicity.
func Replay(ctx context.Context, rdb *redis.Client, dlqStreams []string, qCfg config.QueueConfig, limit int, dryRun bool, log *slog.Logger) error {
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
					continue
				}
			}

			// Atomic XAdd + XDel via Lua script to prevent duplicates on partial failure.
			// ARGV layout: [1]=maxLen, [2]=DLQ msg ID, [3..]=field-value pairs.
			keys := []string{target, dlq}
			args := []any{qCfg.MaxLen, m.ID}
			vals := replayValues(m, payload)
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
				log.Warn("replay XDel returned 0, message was already gone", "id", m.ID)
			}
		}
		log.Info("replay done", "dlq", dlq, "replayed", replayed, "skipped", skipped, "failed", failed)
	}
	return nil
}

// Purge bulk-deletes messages from DLQ streams.
func Purge(ctx context.Context, rdb *redis.Client, streams []string, limit int, dryRun bool, log *slog.Logger) error {
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
	if tp, ok := dlqMsg.Values["_otel_traceparent"]; ok {
		vals["_otel_traceparent"] = tp
	}
	if ts, ok := dlqMsg.Values["_otel_tracestate"]; ok {
		vals["_otel_tracestate"] = ts
	}
	return vals
}
