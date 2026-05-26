package queue

import (
	"os"
	"strconv"
	"time"
)

// Config holds queue configuration loaded from environment variables.
type Config struct {
	Group          string
	Consumer       string
	MaxLen         int64
	DeleteOnAck    bool
	MaxRetries     int
	MaxBackoff     time.Duration
	FetchStream    string
	FetchDLQStream string
	ParseStream    string
	ParseDLQStream string
	AsyncRetry     bool
	AsyncRetryZSet string
}

// LoadConfig reads QUEUE_* environment variables and returns a Config with
// defaults matching go-ingestion/internal/config/config.go.
func LoadConfig() Config {
	return Config{
		Group:          getStr("QUEUE_GROUP", "workers"),
		Consumer:       getStr("QUEUE_CONSUMER", ""),
		MaxLen:         getInt64("QUEUE_MAX_LEN", 10000),
		DeleteOnAck:    getBool("QUEUE_DELETE_ON_ACK", false),
		MaxRetries:     getInt("QUEUE_MAX_RETRIES", 3),
		MaxBackoff:     getDur("QUEUE_MAX_BACKOFF", 30*time.Second),
		FetchStream:    getStr("QUEUE_FETCH_STREAM", "dota2:fetch"),
		FetchDLQStream: getStr("QUEUE_FETCH_DLQ_STREAM", "dota2:fetch:dlq"),
		ParseStream:    getStr("QUEUE_PARSE_STREAM", "dota2:parse"),
		ParseDLQStream: getStr("QUEUE_PARSE_DLQ_STREAM", "dota2:parse:dlq"),
		AsyncRetry:     getBool("QUEUE_ASYNC_RETRY", false),
		AsyncRetryZSet: getStr("QUEUE_ASYNC_RETRY_ZSET", ""),
	}
}

// ──────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────

func getStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getInt64(key string, def int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}

func getBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseBool(v); err == nil {
			return n
		}
	}
	return def
}

func getDur(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if n, err := time.ParseDuration(v); err == nil {
			return n
		}
	}
	return def
}
