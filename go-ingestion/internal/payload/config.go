package payload

import (
	"os"
	"time"
)

// Config holds configuration for the payload package.
type Config struct {
	// KeyPrefix is the Redis key prefix for payload entries.
	KeyPrefix string

	// DefaultTTL is the default TTL for payload entries.
	DefaultTTL time.Duration
}

// LoadConfig loads payload configuration from environment variables.
// Returns a Config with sensible defaults when env vars are not set.
func LoadConfig() Config {
	return Config{
		KeyPrefix:  getEnv("PAYLOAD_KEY_PREFIX", "dota2:payload"),
		DefaultTTL: getDuration("PAYLOAD_DEFAULT_TTL", 24*time.Hour),
	}
}

// getEnv returns the value of the environment variable key, or def if unset.
func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// getDuration returns the parsed time.Duration from the environment variable
// key, or def if the variable is unset or cannot be parsed.
func getDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
