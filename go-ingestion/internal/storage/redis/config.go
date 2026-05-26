package redis

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// LoadConfig loads Redis configuration from REDIS_* environment variables.
// Defaults match the existing internal/config/config.go RedisConfig defaults.
func LoadConfig() Config {
	return Config{
		Addrs:           getAddrs("REDIS_ADDRS"),
		Password:        getStr("REDIS_PASSWORD", ""),
		DB:              getInt("REDIS_DB", 0),
		PoolSize:        getInt("REDIS_POOL_SIZE", 100),
		MinIdleConns:    getInt("REDIS_MIN_IDLE_CONNS", 10),
		MaxActiveConns:  getInt("REDIS_MAX_ACTIVE_CONNS", 0),
		ConnMaxLifetime: getDur("REDIS_CONN_MAX_LIFETIME", 30*time.Minute),
		ConnMaxIdleTime: getDur("REDIS_CONN_MAX_IDLE_TIME", 10*time.Minute),
		DialTimeout:     getDur("REDIS_DIAL_TIMEOUT", 5*time.Second),
		ReadTimeout:     getDur("REDIS_READ_TIMEOUT", 3*time.Second),
		WriteTimeout:    getDur("REDIS_WRITE_TIMEOUT", 3*time.Second),
		ReadOnly:        getBool("REDIS_READ_ONLY", false),
	}
}

// getAddrs reads a comma-separated env var and returns the split, trimmed values.
func getAddrs(key string) []string {
	v := os.Getenv(key)
	if v == "" {
		return []string{"127.0.0.1:6379"}
	}
	parts := strings.Split(v, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

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
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func getBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}
