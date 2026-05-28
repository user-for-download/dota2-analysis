package fetcher

import (
	"os"
	"strconv"
	"time"
)

// EnvConfig holds fetcher configuration loaded exclusively from environment
// variables.  Logger is not included — it is wired at the application level so
// that the fetcher package has no dependency on a specific logging framework.
type EnvConfig struct {
	UpstreamURL     string
	LocalURL        string // NEW — direct URL bypassing proxy pool
	Batch           int
	Block           time.Duration
	HTTPTimeout     time.Duration
	PayloadTTL      time.Duration
	WaitTimeout     time.Duration
	MaxProxyRetries int
	ProxyBackoff    time.Duration
	AllowDirect     bool
}

// LoadConfig reads FETCHER_* environment variables and returns an EnvConfig
// populated with those values (or sensible defaults).  Defaults mirror those
// in config.FetcherConfig so that behaviour is consistent regardless of which
// configuration path the caller chooses.
func LoadConfig() EnvConfig {
	return EnvConfig{
		UpstreamURL:     getStr("FETCHER_UPSTREAM_URL", ""),
		LocalURL:        getStr("FETCHER_UPSTREAM_LOCAL_URL", ""),
		Batch:           getInt("FETCHER_BATCH", 10),
		Block:           getDur("FETCHER_BLOCK", 2*time.Second),
		HTTPTimeout:     getDur("FETCHER_HTTP_TIMEOUT", 30*time.Second),
		PayloadTTL:      getDur("FETCHER_PAYLOAD_TTL", 72*time.Hour),
		WaitTimeout:     getDur("FETCHER_WAIT_TIMEOUT", 5*time.Minute),
		MaxProxyRetries: getInt("FETCHER_MAX_PROXY_RETRIES", 1),
		ProxyBackoff:    getDur("FETCHER_PROXY_BACKOFF", 250*time.Millisecond),
		AllowDirect:     getBool("FETCHER_ALLOW_DIRECT", false),
	}
}

// ---------------------------------------------------------------------------
// Helper helpers — small, self-contained so config.go has zero imports beyond
// the standard library.  No external dependency on the internal/config package.
// ---------------------------------------------------------------------------

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

func getDur(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if n, err := time.ParseDuration(v); err == nil {
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
