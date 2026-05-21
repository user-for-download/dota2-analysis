package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/user-for-download/go-dota2/internal/proxy"
)

type WaitConfig struct {
	MinSize      int
	Timeout      time.Duration
	PollInterval time.Duration
}

func WaitForProxies(ctx context.Context, pool proxy.Pool, cfg WaitConfig, log *slog.Logger) error {
	if cfg.MinSize <= 0 {
		return nil
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Minute
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 2 * time.Second
	}

	deadline := time.Now().Add(cfg.Timeout)
	log.Info("waiting for proxy pool", "min_size", cfg.MinSize, "timeout", cfg.Timeout)

	size, err := pool.Size(ctx)
	if err == nil && size >= cfg.MinSize {
		log.Info("proxy pool ready", "size", size, "min", cfg.MinSize)
		return nil
	}

	var lastSize int
	if err != nil {
		log.Warn("proxy pool initial size check failed", "err", err)
	} else {
		lastSize = int(size)
		log.Debug("proxy pool not ready", "current", lastSize, "required", cfg.MinSize)
	}

	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled: %w", ctx.Err())
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for proxy pool: need %d, have %d (after %s)",
					cfg.MinSize, lastSize, cfg.Timeout)
			}
			size, err := pool.Size(ctx)
			if err != nil {
				log.Warn("proxy pool size check failed", "err", err)
				continue
			}
			lastSize = int(size)
			if lastSize >= cfg.MinSize {
				log.Info("proxy pool ready", "size", lastSize, "min", cfg.MinSize)
				return nil
			}
			log.Debug("proxy pool not ready", "current", lastSize, "required", cfg.MinSize)
		}
	}
}