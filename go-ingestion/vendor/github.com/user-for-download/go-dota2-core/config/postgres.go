package config

import "time"

// PostgresConfig holds connection-pool settings for a PostgreSQL database.
// Both ingestion and analytics use the same fields.
type PostgresConfig struct {
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}
