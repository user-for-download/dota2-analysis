package discovery

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"
)

type mockCycle struct {
	name       string
	interval   time.Duration
	runAtStart bool
	err        error

	mu    sync.Mutex
	calls int
}

func (m *mockCycle) Name() string                { return m.name }
func (m *mockCycle) Interval() time.Duration     { return m.interval }
func (m *mockCycle) RunAtStart() bool            { return m.runAtStart }
func (m *mockCycle) RunOnce(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	return m.err
}

func TestSchedulerRunAtStart(t *testing.T) {
	c1 := &mockCycle{name: "c1", runAtStart: true, interval: 0}
	c2 := &mockCycle{name: "c2", runAtStart: false, interval: 0}

	s := NewScheduler([]Cycle{c1, c2}, slog.Default())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	s.Run(ctx)

	c1.mu.Lock()
	if c1.calls != 1 {
		t.Errorf("c1 calls = %d, want 1", c1.calls)
	}
	c1.mu.Unlock()

	c2.mu.Lock()
	if c2.calls != 0 {
		t.Errorf("c2 calls = %d, want 0", c2.calls)
	}
	c2.mu.Unlock()
}

func TestSchedulerInterval(t *testing.T) {
	c := &mockCycle{name: "c1", runAtStart: false, interval: 10 * time.Millisecond}
	s := NewScheduler([]Cycle{c}, slog.Default())

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	s.Run(ctx)

	c.mu.Lock()
	if c.calls == 0 {
		t.Errorf("expected at least 1 call, got %d", c.calls)
	}
	c.mu.Unlock()
}

func TestSchedulerError(t *testing.T) {
	c := &mockCycle{name: "c1", runAtStart: true, interval: 0, err: errors.New("boom")}
	s := NewScheduler([]Cycle{c}, slog.Default())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	s.Run(ctx) // Should log error and not panic
}

func TestJitter(t *testing.T) {
	if jitter(0) != 0 {
		t.Errorf("jitter(0) != 0")
	}
	j := jitter(100 * time.Millisecond)
	if j < 100*time.Millisecond || j > 110*time.Millisecond {
		t.Errorf("jitter out of bounds: %v", j)
	}
}