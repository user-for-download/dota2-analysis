package redisseen

import (
	"context"
	"fmt"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/dedup"
)

type Config struct {
	KeyPrefix      string
	TTL            time.Duration
	UseBloom       bool
	BloomCapacity  int64
	BloomErrorRate float64
}

type Seen struct {
	rdb      *goredis.Client
	cfg      Config
	keys     keys
	bloomKey string
}

func New(rdb *goredis.Client, cfg Config) (*Seen, error) {
	if rdb == nil {
		return nil, fmt.Errorf("redisseen: nil redis client")
	}
	if cfg.KeyPrefix == "" {
		return nil, fmt.Errorf("redisseen: KeyPrefix is required")
	}
	s := &Seen{
		rdb:      rdb,
		cfg:      cfg,
		keys:     keys{prefix: cfg.KeyPrefix},
		bloomKey: cfg.KeyPrefix + ":bloom",
	}

	if cfg.UseBloom {
		// Explicitly reserve the bloom filter with configured error rate and capacity.
		// This prevents Redis from defaulting to a tiny 1,000-item filter.
		err := rdb.Do(context.Background(), "BF.RESERVE", s.bloomKey, cfg.BloomErrorRate, cfg.BloomCapacity).Err()
		if err != nil && !isAlreadyExists(err) {
			if isUnknownCommand(err) {
				return nil, fmt.Errorf("bf.reserve: RedisBloom module not loaded: %w", err)
			}
			return nil, fmt.Errorf("bf.reserve: %w", err)
		}
	}

	return s, nil
}

func isAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "BUSYKEY")
}

func isUnknownCommand(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "unknown command")
}

var _ dedup.Seen = (*Seen)(nil)

func (s *Seen) MarkSeen(ctx context.Context, key string) (bool, error) {
	if s.cfg.UseBloom {
		// BF.ADD returns 1 if item was newly added, 0 if it already existed.
		// MarkSeen returns (alreadySeen bool), so we return added == 0.
		added, err := s.rdb.Do(ctx, "BF.ADD", s.bloomKey, key).Int64()
		if err != nil {
			return false, fmt.Errorf("bf.add: %w", err)
		}
		return added == 0, nil
	}
	if s.cfg.TTL <= 0 {
		added, err := s.rdb.SAdd(ctx, s.keys.set(), key).Result()
		if err != nil {
			return false, fmt.Errorf("sadd: %w", err)
		}
		return added == 0, nil
	}
	added, err := s.rdb.SetNX(ctx, s.keys.seenKey(key), "1", s.cfg.TTL).Result()
	if err != nil {
		return false, fmt.Errorf("setnx: %w", err)
	}
	return !added, nil
}

func (s *Seen) IsSeen(ctx context.Context, key string) (bool, error) {
	if s.cfg.UseBloom {
		exists, err := s.rdb.Do(ctx, "BF.EXISTS", s.bloomKey, key).Int64()
		if err != nil {
			return false, fmt.Errorf("bf.exists: %w", err)
		}
		return exists > 0, nil
	}
	if s.cfg.TTL <= 0 {
		exists, err := s.rdb.SIsMember(ctx, s.keys.set(), key).Result()
		if err != nil {
			return false, fmt.Errorf("sismember: %w", err)
		}
		return exists, nil
	}
	exists, err := s.rdb.Exists(ctx, s.keys.seenKey(key)).Result()
	if err != nil {
		return false, fmt.Errorf("exists: %w", err)
	}
	return exists > 0, nil
}

func (s *Seen) CheckBatch(ctx context.Context, keys []string) ([]bool, error) {
	if s.cfg.UseBloom {
		args := make([]any, 0, 2+len(keys))
		args = append(args, "BF.MEXISTS", s.bloomKey)
		for _, k := range keys {
			args = append(args, k)
		}
		res, err := s.rdb.Do(ctx, args...).Slice()
		if err != nil {
			return nil, fmt.Errorf("bf.mexists: %w", err)
		}
		out := make([]bool, len(keys))
		for i, v := range res {
			if iv, ok := v.(int64); ok {
				out[i] = iv > 0
			}
		}
		return out, nil
	}
	if s.cfg.TTL <= 0 {
		return s.smismember(ctx, keys)
	}
	out := make([]bool, len(keys))
	for i, k := range keys {
		exists, err := s.rdb.Exists(ctx, s.keys.seenKey(k)).Result()
		if err != nil {
			return nil, fmt.Errorf("exists: %w", err)
		}
		out[i] = exists > 0
	}
	return out, nil
}

func (s *Seen) smismember(ctx context.Context, keys []string) ([]bool, error) {
	pipe := s.rdb.Pipeline()
	cmds := make([]*goredis.BoolCmd, len(keys))
	for i, k := range keys {
		cmds[i] = pipe.SIsMember(ctx, s.keys.set(), k)
	}
	_, err := pipe.Exec(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]bool, len(keys))
	for i, cmd := range cmds {
		out[i] = cmd.Val()
	}
	return out, nil
}