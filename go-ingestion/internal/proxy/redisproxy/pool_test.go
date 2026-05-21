package redisproxy

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/user-for-download/go-dota2/internal/proxy"
)

func newTestPool(t *testing.T) (*Pool, *goredis.Client, func()) {
	t.Helper()
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("set TEST_REDIS_ADDR to run")
	}
	rdb := goredis.NewClient(&goredis.Options{Addr: addr})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		t.Skipf("redis ping failed: %v", err)
	}

	prefix := "test:proxy:" + time.Now().Format("150405.000000000")
	p, err := New(rdb, Config{
		KeyPrefix: prefix,
		RateLimit: proxy.RateLimit{RatePerSec: 100, Burst: 100, Window: time.Second},
		Ranking:   proxy.Ranking{InitialWeight: 100, SuccessBoost: 1, FailurePenalty: 5},
	})
	if err != nil {
		_ = rdb.Close()
		t.Fatalf("New: %v", err)
	}

	cleanup := func() {
		bg := context.Background()
		iter := rdb.Scan(bg, 0, prefix+":*", 1000).Iterator()
		for iter.Next(bg) {
			_ = rdb.Del(bg, iter.Val()).Err()
		}
		_ = rdb.Close()
	}
	return p, rdb, cleanup
}

func TestRedisProxyAcquireRelease(t *testing.T) {
	p, _, cleanup := newTestPool(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := p.Replace(ctx, []string{"http://a", "http://b"}); err != nil {
		t.Fatalf("Replace: %v", err)
	}

	n, _ := p.Size(ctx)
	if n != 2 {
		t.Errorf("size = %d, want 2", n)
	}

	l, err := p.Acquire(ctx, 5*time.Second)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if l.URL == "" {
		t.Error("empty lease URL")
	}
	if err := l.Release(ctx); err != nil {
		t.Errorf("Release: %v", err)
	}
}

func TestRedisProxyEmptyPool(t *testing.T) {
	p, _, cleanup := newTestPool(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_ = p.Replace(ctx, nil)

	if _, err := p.Acquire(ctx, time.Second); !errors.Is(err, proxy.ErrNoProxy) {
		t.Errorf("err = %v, want ErrNoProxy", err)
	}
}

func TestRedisProxyRateLimit(t *testing.T) {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("set TEST_REDIS_ADDR to run")
	}

	rdb := goredis.NewClient(&goredis.Options{Addr: addr})
	defer rdb.Close()

	p, err := New(rdb, Config{
		KeyPrefix: "test:ratelimit:" + time.Now().Format("150405.000000000"),
		RateLimit: proxy.RateLimit{RatePerSec: 1, Burst: 1, Window: time.Second},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()
	_ = p.Replace(ctx, []string{"http://a"})

	l1, err := p.Acquire(ctx, time.Second)
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}
	_ = l1.Release(ctx)

	_, err = p.Acquire(ctx, time.Second)
	if !errors.Is(err, proxy.ErrRateLimited) {
		t.Errorf("second Acquire err = %v, want ErrRateLimited", err)
	}
}

func TestRedisProxyRecordSuccessAndFailure(t *testing.T) {
	p, _, cleanup := newTestPool(t)
	defer cleanup()

	ctx := context.Background()
	_ = p.Replace(ctx, []string{"http://success", "http://fail"})

	// test success
	l1, err := p.Acquire(ctx, 5*time.Second)
	if err != nil { t.Fatalf("Acquire 1: %v", err) }
	
	l1.MarkSuccess(ctx)
	_ = l1.Release(ctx)

	// test failure
	l2, err := p.Acquire(ctx, 5*time.Second)
	if err != nil { t.Fatalf("Acquire 2: %v", err) }
	
	l2.MarkFailure(ctx, errors.New("boom"))
}

func TestRedisProxyAdd(t *testing.T) {
	p, _, cleanup := newTestPool(t)
	defer cleanup()

	ctx := context.Background()
	if err := p.Add(ctx, []string{"http://new"}); err != nil {
		t.Errorf("Add: %v", err)
	}

	n, _ := p.Size(ctx)
	if n != 1 {
		t.Errorf("Size = %d, want 1", n)
	}
}
