package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

type keys struct {
	prefix string
}

func (k keys) done(source string) string { return k.prefix + ":bootstrap:done:" + source }

type Config struct {
	Prefix         string
	TTL            time.Duration
	ForceBootstrap bool
}

type Marker struct {
	rdb  *goredis.Client
	cfg  Config
	keys keys
}

func New(rdb *goredis.Client, cfg Config) *Marker {
	if cfg.TTL <= 0 {
		cfg.TTL = 30 * 24 * time.Hour
	}
	return &Marker{
		rdb:  rdb,
		cfg:  cfg,
		keys: keys{prefix: cfg.Prefix},
	}
}

func (m *Marker) Done(ctx context.Context, sourceName string) (bool, error) {
	if m.cfg.ForceBootstrap {
		return false, nil
	}
	key := m.keys.done(sourceName)
	exists, err := m.rdb.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("exists: %w", err)
	}
	return exists == 1, nil
}

func (m *Marker) MarkDone(ctx context.Context, sourceName string) error {
	key := m.keys.done(sourceName)
	if err := m.rdb.Set(ctx, key, "1", m.cfg.TTL).Err(); err != nil {
		return fmt.Errorf("set: %w", err)
	}
	return nil
}
