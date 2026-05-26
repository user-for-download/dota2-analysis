package proxy

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Hold               time.Duration
	MinPoolSize        int
	KeyPrefix          string
	RateLimitPerSec    int
	RateLimitBurst     int
	RateLimitWindow    time.Duration
	RankingInitial     float64
	RankingSuccess     float64
	RankingFailure     float64
	MaxFailures        int
	SeedFile           string
	CanaryURL          string
	RemoteURL          string
	ValidateTimeout    time.Duration
	ValidateParallel   int
	ValidateChunkSize  int
	ValidateMinPublish int
	RefreshInterval    time.Duration
}

func LoadConfig() Config {
	return Config{
		Hold:               getDur("PROXY_HOLD", 30*time.Second),
		MinPoolSize:        getInt("PROXY_MIN_POOL_SIZE", 0),
		KeyPrefix:          getStr("PROXY_KEY_PREFIX", "dota2:proxy"),
		RateLimitPerSec:    getInt("PROXY_RATE_LIMIT_PER_SEC", 0),
		RateLimitBurst:     getInt("PROXY_RATE_LIMIT_BURST", 0),
		RateLimitWindow:    getDur("PROXY_RATE_LIMIT_WINDOW", 1*time.Second),
		RankingInitial:     getFloat("PROXY_RANKING_INITIAL", 100),
		RankingSuccess:     getFloat("PROXY_RANKING_SUCCESS", 1),
		RankingFailure:     getFloat("PROXY_RANKING_FAILURE", 5),
		MaxFailures:        getInt("PROXY_MAX_FAILURES", 5),
		SeedFile:           getStr("PROXY_SEED_FILE", "proxy.txt"),
		CanaryURL:          getStr("PROXY_CANARY_URL", "https://api.ipify.org"),
		RemoteURL:          getStr("PROXY_REMOTE_URL", ""),
		ValidateTimeout:    getDur("PROXY_VALIDATE_TIMEOUT", 10*time.Second),
		ValidateParallel:   getInt("PROXY_VALIDATE_PARALLEL", 50),
		ValidateChunkSize:  getInt("PROXY_VALIDATE_CHUNK_SIZE", 100),
		ValidateMinPublish: getInt("PROXY_VALIDATE_MIN_PUBLISH", 0),
		RefreshInterval:    getDur("PROXY_REFRESH_INTERVAL", 0),
	}
}

func getStr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func getInt(k string, d int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return d
}

func getFloat(k string, d float64) float64 {
	if v := os.Getenv(k); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return d
}

func getDur(k string, d time.Duration) time.Duration {
	if v := os.Getenv(k); v != "" {
		if t, err := time.ParseDuration(v); err == nil {
			return t
		}
	}
	return d
}
