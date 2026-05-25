package redisproxy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	mathrand "math/rand"
	"os"
	"strconv"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/proxy"
)

type Config struct {
	KeyPrefix    string
	RateLimit  proxy.RateLimit
	Ranking   proxy.Ranking
	MaxFailures int
	Logger    *slog.Logger
}

type Pool struct {
	rdb  *goredis.Client
	cfg  Config
	keys keys
	log  *slog.Logger

	scriptAcquire       *goredis.Script
	scriptRelease       *goredis.Script
	scriptRateLimit    *goredis.Script
	scriptRecordSuccess *goredis.Script
	scriptRecordFailure *goredis.Script

	cooldownInterval time.Duration
}

func New(rdb *goredis.Client, cfg Config) (*Pool, error) {
	if rdb == nil {
		return nil, fmt.Errorf("redisproxy: nil redis client")
	}
	if cfg.KeyPrefix == "" {
		return nil, fmt.Errorf("redisproxy: KeyPrefix is required")
	}
	cfg.Ranking = cfg.Ranking.WithDefaults()
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	cooldownInterval := 5 * time.Minute
	if v := os.Getenv("PROXY_COOLDOWN_CHECK_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cooldownInterval = d
		}
	}
	return &Pool{
		rdb:             rdb,
		cfg:             cfg,
		keys:            keys{prefix: cfg.KeyPrefix},
		log:             log.With("component", "redisproxy", "prefix", cfg.KeyPrefix),
		scriptAcquire:    goredis.NewScript(luaAcquire),
		scriptRelease:    goredis.NewScript(luaRelease),
		scriptRateLimit: goredis.NewScript(luaRateLimit),
		scriptRecordSuccess: goredis.NewScript(luaRecordSuccess),
		scriptRecordFailure: goredis.NewScript(luaRecordFailure),
		cooldownInterval: cooldownInterval,
	}, nil
}

var _ proxy.Pool = (*Pool)(nil)

func (p *Pool) Acquire(ctx context.Context, hold time.Duration) (*proxy.Lease, error) {
	if hold <= 0 {
		hold = 30 * time.Second
	}

	if !p.cfg.RateLimit.Disabled() {
		window := p.cfg.RateLimit.Window
		if window <= 0 {
			window = time.Second
		}
		windowMs := int64(window / time.Millisecond)
		if windowMs < 1 {
			windowMs = 1000
		}
		ok, err := p.scriptRateLimit.Run(
			ctx, p.rdb,
			[]string{p.keys.limiter()},
			p.cfg.RateLimit.Burst,
			windowMs,
		).Int64()
		if err != nil {
			return nil, fmt.Errorf("rate-limit script: %w", err)
		}
		if ok == 0 {
			return nil, proxy.ErrRateLimited
		}
	}

	token, err := newToken()
	if err != nil {
		return nil, err
	}
	leaseKey := p.keys.lease(token)
	ttlSec := int64(hold.Seconds())
	if ttlSec < 1 {
		ttlSec = 1
	}

	res, err := p.scriptAcquire.Run(
		ctx, p.rdb,
		[]string{p.keys.set(), p.keys.leased(), leaseKey},
		ttlSec, token, 20, mathrand.Int63(),
	).Result()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return nil, proxy.ErrNoProxy
		}
		return nil, fmt.Errorf("acquire script: %w", err)
	}
	url, ok := res.(string)
	if !ok || url == "" {
		return nil, proxy.ErrNoProxy
	}

	return proxy.NewLease(
		url,
		p.releaseLease(token),
		p.recordSuccess(url),
		p.recordFailure(url),
	), nil
}

func (p *Pool) Size(ctx context.Context) (int, error) {
	n, err := p.rdb.ZCard(ctx, p.keys.set()).Result()
	if err != nil {
		return 0, fmt.Errorf("zcard: %w", err)
	}
	return int(n), nil
}

func (p *Pool) Replace(ctx context.Context, healthy []string) error {
	current, err := p.rdb.ZRange(ctx, p.keys.set(), 0, -1).Result()
	if err != nil {
		return fmt.Errorf("get current proxies: %w", err)
	}

	newSet := make(map[string]bool, len(healthy))
	for _, url := range healthy {
		newSet[url] = true
	}

	var toDelete []string
	for _, url := range current {
		if !newSet[url] {
			toDelete = append(toDelete, p.keys.stats(url))
		}
	}

	pipe := p.rdb.TxPipeline()
	pipe.Del(ctx, p.keys.set())
	if len(healthy) > 0 {
		members := make([]goredis.Z, 0, len(healthy))
		for _, url := range healthy {
			members = append(members, goredis.Z{
				Score:  p.cfg.Ranking.InitialWeight,
				Member: url,
			})
		}
		pipe.ZAdd(ctx, p.keys.set(), members...)
	}
	if len(toDelete) > 0 {
		pipe.Del(ctx, toDelete...)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("replace pipeline: %w", err)
	}
	return nil
}

func (p *Pool) Add(ctx context.Context, healthy []string) error {
	if len(healthy) == 0 {
		return nil
	}
	members := make([]goredis.Z, 0, len(healthy))
	for _, url := range healthy {
		members = append(members, goredis.Z{
			Score:  p.cfg.Ranking.InitialWeight,
			Member: url,
		})
	}
	if err := p.rdb.ZAddNX(ctx, p.keys.set(), members...).Err(); err != nil {
		return fmt.Errorf("zadd nx: %w", err)
	}
	return nil
}

func (p *Pool) StartCooldownRecovery(ctx context.Context) {
	go p.runCooldownRecovery(ctx)
}

func (p *Pool) runCooldownRecovery(ctx context.Context) {
	ticker := time.NewTicker(p.cooldownInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.recoverFromCooldown(ctx)
		}
	}
}

func (p *Pool) recoverFromCooldown(ctx context.Context) {
	now := float64(time.Now().Unix())
	res, err := p.rdb.ZRangeByScoreWithScores(ctx, p.keys.cooldown(), &goredis.ZRangeBy{
		Min:   "-inf",
		Max:   fmt.Sprintf("%f", now),
		Count: 50,
	}).Result()
	if err != nil {
		p.log.Debug("cooldown recovery: zrangebyscore failed", "err", err)
		return
	}
	if len(res) == 0 {
		return
	}

	recovered := 0
	for _, z := range res {
		url, ok := z.Member.(string)
		if !ok {
			continue
		}
		statsKey := p.keys.stats(url)
		exists, err := p.rdb.Exists(ctx, statsKey).Result()
		if err != nil {
			p.log.Warn("cooldown recovery: stats check failed", "url", url, "err", err)
			continue
		}
		if exists > 0 {
			pipe := p.rdb.Pipeline()
			pipe.ZAdd(ctx, p.keys.set(), goredis.Z{Score: p.cfg.Ranking.InitialWeight, Member: url})
			pipe.HSet(ctx, statsKey, "consecutive_fail", 0)
			pipe.ZRem(ctx, p.keys.cooldown(), url)
			if _, err := pipe.Exec(ctx); err != nil {
				p.log.Warn("cooldown recovery: failed to restore proxy", "url", url, "err", err)
				continue
			}
			recovered++
			p.log.Info("proxy recovered from cooldown", "url", url)
		} else {
			if _, err := p.rdb.ZRem(ctx, p.keys.cooldown(), url).Result(); err != nil {
				p.log.Warn("cooldown cleanup failed", "url", url, "err", err)
			}
		}
	}
	if recovered > 0 {
		p.log.Info("cooldown recovery done", "recovered", recovered)
	}
}

func (p *Pool) releaseLease(token string) func(context.Context) error {
	return func(ctx context.Context) error {
		_, err := p.scriptRelease.Run(
			ctx, p.rdb,
			[]string{p.keys.leased(), p.keys.lease(token)},
		).Int64()
		if err != nil && !errors.Is(err, goredis.Nil) {
			return fmt.Errorf("release script: %w", err)
		}
		return nil
	}
}

func (p *Pool) recordSuccess(url string) func(context.Context) error {
	return func(ctx context.Context) error {
		_, err := p.scriptRecordSuccess.Run(
			ctx, p.rdb,
			[]string{p.keys.set(), p.keys.stats(url)},
			url, p.cfg.Ranking.SuccessBoost,
		).Int64()
		if err != nil && !errors.Is(err, goredis.Nil) {
			p.log.Warn("record success failed", "url", url, "err", err)
			return err
		}
		return nil
	}
}

func (p *Pool) recordFailure(url string) func(context.Context, error) error {
	return func(ctx context.Context, cause error) error {
		msg := "unspecified"
		if cause != nil {
			msg = cause.Error()
		}
		coolSecs := int64(300)
		if v := os.Getenv("PROXY_COOLDOWN_SECONDS"); v != "" {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
				coolSecs = n
			}
		}
		res, err := p.scriptRecordFailure.Run(
			ctx, p.rdb,
			[]string{p.keys.set(), p.keys.stats(url), p.keys.cooldown(), p.keys.cooldownEntry(url)},
			url, p.cfg.Ranking.FailurePenalty, p.cfg.MaxFailures, msg, coolSecs,
		).Int64()
		if err != nil && !errors.Is(err, goredis.Nil) {
			p.log.Warn("record failure failed", "url", url, "err", err)
			return err
		}
		if res == 1 {
			p.log.Info("proxy evicted to cooldown after consecutive failures",
				"url", url, "threshold", p.cfg.MaxFailures, "cooldown_seconds", coolSecs)
		}
		return nil
	}
}

func newToken() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("token rand: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}
