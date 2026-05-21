package inmem

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/user-for-download/go-dota2/internal/queue"
)

type Queue struct {
	mu       sync.Mutex
	pending  []queue.Task
	inFlight map[string]queue.Task

	policy queue.RetryPolicy
	dlq    []queue.Task

	seq int
}

func New(policy queue.RetryPolicy) *Queue {
	return &Queue{
		inFlight: make(map[string]queue.Task),
		policy:   policy,
	}
}

var _ queue.Queue = (*Queue)(nil)

func (q *Queue) Push(_ context.Context, payload []byte) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.pending = append(q.pending, queue.Task{
		ID:      q.nextID(),
		Payload: append([]byte(nil), payload...),
	})
	return nil
}

func (q *Queue) Pop(ctx context.Context, batch int, block time.Duration) ([]queue.Task, error) {
	deadline := time.Now().Add(block)
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		q.mu.Lock()
		if len(q.pending) > 0 {
			if batch > len(q.pending) {
				batch = len(q.pending)
			}
			out := make([]queue.Task, batch)
			copy(out, q.pending[:batch])
			q.pending = q.pending[batch:]
			for _, t := range out {
				q.inFlight[t.ID] = t
			}
			q.mu.Unlock()
			return out, nil
		}
		q.mu.Unlock()

		if !time.Now().Before(deadline) {
			return nil, queue.ErrEmpty
		}

		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func (q *Queue) Ack(_ context.Context, taskID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.inFlight, taskID)
	return nil
}

func (q *Queue) Retry(_ context.Context, t queue.Task, _ string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	delete(q.inFlight, t.ID)
	t.RetryCount++

	if q.policy.ShouldDLQ(t.RetryCount) {
		q.dlq = append(q.dlq, t)
		return nil
	}
	t.ID = q.nextID()
	q.pending = append(q.pending, t)
	return nil
}

func (q *Queue) RecoverStale(_ context.Context, _ time.Duration, _ int) ([]queue.Task, error) {
	return nil, nil
}

func (q *Queue) DLQ() []queue.Task {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]queue.Task, len(q.dlq))
	copy(out, q.dlq)
	return out
}

func (q *Queue) PendingLen() int64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	return int64(len(q.pending))
}

func (q *Queue) InFlightLen() int64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	return int64(len(q.inFlight))
}

func (q *Queue) nextID() string {
	q.seq++
	return fmt.Sprintf("inmem-%d", q.seq)
}
