package parser

import (
	"os"
	"strconv"
	"time"
)

// LoadConfig reads PARSER_* environment variables and returns a Config with
// sensible defaults matching internal/config/config.go.
func LoadConfig() Config {
	return Config{
		Batch:                        getInt("PARSER_BATCH", 10),
		Block:                        getDur("PARSER_BLOCK", 2*time.Second),
		PartitionMaintenanceInterval: getDur("PARSER_PARTITION_MAINTENANCE_INTERVAL", 24*time.Hour),
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

func getDur(k string, d time.Duration) time.Duration {
	if v := os.Getenv(k); v != "" {
		if t, err := time.ParseDuration(v); err == nil {
			return t
		}
	}
	return d
}
