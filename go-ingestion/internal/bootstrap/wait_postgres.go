package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	coreconfig "github.com/user-for-download/dota2-analysis/go-core/config"
)

func WaitForPostgres(ctx context.Context, cfg coreconfig.PostgresConfig, wait WaitConfig, log *slog.Logger) (*pgxpool.Pool, error) {
	if wait.PollInterval <= 0 {
		wait.PollInterval = 30 * time.Second
	}

	var deadline time.Time
	if wait.Timeout > 0 {
		deadline = time.Now().Add(wait.Timeout)
	}
	// Note: If wait.Timeout <= 0, deadline remains zero and retries run indefinitely
	// until either the context is cancelled or Postgres becomes available.

	log.Info("waiting for postgres",
		"poll_interval", wait.PollInterval,
		"timeout", wait.Timeout,
	)

	attempt := 0
	for {
		attempt++
		db, err := Postgres(ctx, cfg, log)
		if err == nil {
			log.Info("postgres ready", "attempts", attempt)
			return db, nil
		}

		log.Warn("postgres not ready; will retry",
			"attempt", attempt,
			"interval", wait.PollInterval,
			"err", err,
		)

		if !deadline.IsZero() && time.Now().After(deadline) {
			return nil, fmt.Errorf("timeout waiting for postgres after %s (%d attempts): %w",
				wait.Timeout, attempt, err)
		}

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("context cancelled waiting for postgres: %w", ctx.Err())
		case <-time.After(wait.PollInterval):
		}
	}
}