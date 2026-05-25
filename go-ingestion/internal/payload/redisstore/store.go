package redisstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/payload"
)

type Config struct {
	KeyPrefix  string
	DefaultTTL time.Duration
}

type Store struct {
	rdb  *goredis.Client
	cfg  Config
	keys keys
}

func New(rdb *goredis.Client, cfg Config) (*Store, error) {
	if rdb == nil {
		return nil, fmt.Errorf("redispayload: nil redis client")
	}
	if cfg.KeyPrefix == "" {
		return nil, fmt.Errorf("redispayload: KeyPrefix is required")
	}
	return &Store{
		rdb:  rdb,
		cfg:  cfg,
		keys: keys{prefix: cfg.KeyPrefix},
	}, nil
}

var _ payload.Store = (*Store)(nil)

func (s *Store) Put(ctx context.Context, key string, data []byte, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = s.cfg.DefaultTTL
	}
	if err := s.rdb.Set(ctx, s.keys.blob(key), data, ttl).Err(); err != nil {
		return fmt.Errorf("payload set: %w", err)
	}
	return nil
}

func (s *Store) Get(ctx context.Context, key string) ([]byte, error) {
	b, err := s.rdb.Get(ctx, s.keys.blob(key)).Bytes()
	if errors.Is(err, goredis.Nil) {
		return nil, payload.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("payload get: %w", err)
	}
	return b, nil
}

func (s *Store) Delete(ctx context.Context, key string) error {
	if err := s.rdb.Del(ctx, s.keys.blob(key)).Err(); err != nil {
		return fmt.Errorf("payload del: %w", err)
	}
	return nil
}

func (s *Store) ExtendTTL(ctx context.Context, key string, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = s.cfg.DefaultTTL
	}
	if err := s.rdb.Expire(ctx, s.keys.blob(key), ttl).Err(); err != nil {
		return fmt.Errorf("payload expire: %w", err)
	}
	return nil
}
