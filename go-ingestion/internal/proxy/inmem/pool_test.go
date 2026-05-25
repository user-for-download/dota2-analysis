package inmem

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/proxy"
)

func TestAcquireRoundRobin(t *testing.T) {
	p := New([]string{"a", "b", "c"})
	ctx := context.Background()

	seen := map[string]int{}
	for i := 0; i < 3; i++ {
		l, err := p.Acquire(ctx, time.Second)
		if err != nil {
			t.Fatalf("Acquire %d: %v", i, err)
		}
		seen[l.URL]++
		if err := l.Release(ctx); err != nil {
			t.Fatalf("Release: %v", err)
		}
	}
	if len(seen) != 3 {
		t.Errorf("expected each proxy once, got %v", seen)
	}
}

func TestAcquireExhausts(t *testing.T) {
	p := New([]string{"only"})
	ctx := context.Background()

	l1, err := p.Acquire(ctx, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l1.Release(ctx) }()

	if _, err := p.Acquire(ctx, time.Second); !errors.Is(err, proxy.ErrNoProxy) {
		t.Errorf("err = %v, want ErrNoProxy", err)
	}
}

func TestAcquireEmpty(t *testing.T) {
	p := New(nil)
	ctx := context.Background()
	if _, err := p.Acquire(ctx, time.Second); !errors.Is(err, proxy.ErrNoProxy) {
		t.Errorf("err = %v, want ErrNoProxy", err)
	}
}

func TestReleaseIdempotent(t *testing.T) {
	p := New([]string{"a"})
	ctx := context.Background()

	l, err := p.Acquire(ctx, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Release(ctx); err != nil {
		t.Fatal(err)
	}
	if err := l.Release(ctx); err != nil {
		t.Fatal(err)
	}
	if p.LeasedCount() != 0 {
		t.Errorf("leased = %d, want 0", p.LeasedCount())
	}
}

func TestMarkSuccessFailure(t *testing.T) {
	p := New([]string{"a"})
	ctx := context.Background()

	l, _ := p.Acquire(ctx, time.Second)
	l.MarkSuccess(ctx)
	l.MarkSuccess(ctx)
	l.MarkFailure(ctx, errors.New("boom"))
	_ = l.Release(ctx)

	s := p.StatsFor("a")
	if s.Successes != 2 || s.Failures != 1 {
		t.Errorf("stats = %+v, want {2 1}", s)
	}
}

func TestReplace(t *testing.T) {
	p := New([]string{"a", "b"})
	ctx := context.Background()

	if err := p.Replace(ctx, []string{"x", "y", "z"}); err != nil {
		t.Fatal(err)
	}
	n, _ := p.Size(ctx)
	if n != 3 {
		t.Errorf("size = %d, want 3", n)
	}
}

func TestReplacePreservesActiveLease(t *testing.T) {
	p := New([]string{"a"})
	ctx := context.Background()

	l, _ := p.Acquire(ctx, time.Second)
	if err := p.Replace(ctx, []string{"b"}); err != nil {
		t.Fatal(err)
	}
	if err := l.Release(ctx); err != nil {
		t.Errorf("release after replace: %v", err)
	}
}

func TestAcquireRespectsContext(t *testing.T) {
	p := New([]string{"a"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := p.Acquire(ctx, time.Second); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}
