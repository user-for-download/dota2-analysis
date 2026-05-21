package gate

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

type RunOutcome struct {
	Success bool
	Err     string
	At      time.Time
}

type RunGate interface {
	ShouldRun(ctx context.Context, name string) (bool, error)
	RecordRun(ctx context.Context, name string, outcome RunOutcome) error
}

type Mode int

const (
	Interval Mode = iota
	Once
)

type Config struct {
	Prefix string
	MinAge time.Duration
	TTL    time.Duration
	Mode   Mode
	Client *goredis.Client
}

type Gate struct {
	cfg Config
}

func New(cfg Config) *Gate {
	return &Gate{cfg: cfg}
}

func (g *Gate) ShouldRun(ctx context.Context, name string) (bool, error) {
	key := g.key(name)

	if g.cfg.Mode == Once {
		exists, err := g.cfg.Client.Exists(ctx, key).Result()
		if err != nil {
			return false, fmt.Errorf("exists: %w", err)
		}
		return exists == 0, nil
	}

	lastStr, err := g.cfg.Client.Get(ctx, key).Result()
	if err == goredis.Nil {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("get last run: %w", err)
	}

	lastAt, err := time.Parse(time.RFC3339, lastStr)
	if err != nil {
		return true, nil
	}
	return time.Since(lastAt) >= g.cfg.MinAge, nil
}

func (g *Gate) RecordRun(ctx context.Context, name string, outcome RunOutcome) error {
	if !outcome.Success {
		return nil
	}
	key := g.key(name)

	var ttl time.Duration
	if g.cfg.Mode == Once {
		ttl = g.cfg.TTL
	} else {
		ttl = g.cfg.MinAge
	}
	return g.cfg.Client.Set(ctx, key, outcome.At.Format(time.RFC3339), ttl).Err()
}

func (g *Gate) key(name string) string {
	suffix := "run"
	if g.cfg.Mode == Once {
		suffix = "done"
	}
	return g.cfg.Prefix + ":" + suffix + ":" + name
}

type Always struct{}

func (Always) ShouldRun(context.Context, string) (bool, error) { return true, nil }
func (Always) RecordRun(context.Context, string, RunOutcome) error { return nil }

var _ RunGate = (*Gate)(nil)
var _ RunGate = Always{}