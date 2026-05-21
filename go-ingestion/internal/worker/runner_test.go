package worker

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/user-for-download/go-dota2/internal/metrics"
	"github.com/user-for-download/go-dota2/internal/queue"
)

type mockQueue struct {
	tasks []queue.Task
	err   error
	popCalls int
}

func (m *mockQueue) Push(ctx context.Context, payload []byte) error { return nil }
func (m *mockQueue) Pop(ctx context.Context, count int, block time.Duration) ([]queue.Task, error) {
	m.popCalls++
	if m.err != nil {
		return nil, m.err
	}
	if len(m.tasks) > 0 {
		tasks := m.tasks
		m.tasks = nil
		return tasks, nil
	}
	// Simulate blocking to avoid tight loop in tests when empty
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(10 * time.Millisecond):
		return nil, queue.ErrEmpty
	}
}
func (m *mockQueue) Ack(ctx context.Context, id string) error { return nil }
func (m *mockQueue) Retry(ctx context.Context, task queue.Task, reason string) error { return nil }
func (m *mockQueue) RecoverStale(ctx context.Context, idleFor time.Duration, max int) ([]queue.Task, error) { return nil, nil }
func (m *mockQueue) PendingLen() int64  { return 0 }
func (m *mockQueue) InFlightLen() int64 { return 0 }

type mockHandler struct {
	err error
	processed []queue.Task
}

func (h *mockHandler) Process(ctx context.Context, t queue.Task) (Result, error) {
	h.processed = append(h.processed, t)
	return ResultSuccess, h.err
}

func TestRun(t *testing.T) {
	q := &mockQueue{
		tasks: []queue.Task{{ID: "1", Payload: []byte("foo")}},
	}
	h := &mockHandler{}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := Run(ctx, q, Config{Batch: 1, Block: time.Millisecond}, metrics.StageFetch, h)
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Errorf("expected context cancellation, got %v", err)
	}

	if len(h.processed) != 1 {
		t.Errorf("expected 1 task processed, got %d", len(h.processed))
	}
}

type recoverStaleQueue struct {
	*mockQueue
	recovered bool
}

func (r *recoverStaleQueue) RecoverStale(ctx context.Context, idleFor time.Duration, max int) ([]queue.Task, error) {
	r.recovered = true
	return nil, nil
}

func TestRun_RecoverStale(t *testing.T) {
	q := &recoverStaleQueue{mockQueue: &mockQueue{}}
	h := &mockHandler{}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	cfg := Config{
		Batch: 1,
		Block: time.Millisecond,
		RecoverStale: true,
		Logger: slog.Default(),
	}
	_ = Run(ctx, q, cfg, metrics.StageFetch, h)
}
