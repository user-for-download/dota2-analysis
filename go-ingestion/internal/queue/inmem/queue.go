package inmem

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/queue"
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

var _ queue.Publisher = (*Queue)(nil)
var _ queue.Subscriber = (*Queue)(nil)

// Publish implements queue.Publisher.
func (q *Queue) Publish(ctx context.Context, msg queue.Message) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.pending = append(q.pending, queue.Task{
		Message: queue.Message{
			ID:      q.nextID(),
			Payload: append([]byte(nil), msg.Payload...),
			Headers: msg.Headers,
		},
	})
	return nil
}

// Push is a convenience wrapper for tests — creates a Message from raw bytes.
func (q *Queue) Push(ctx context.Context, payload []byte) error {
	return q.Publish(ctx, queue.Message{Payload: payload})
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

// Subscribe implements queue.Subscriber.  Polls the queue, calls handler
// per message, and Acks or Retries as appropriate.  Blocks until ctx
// is cancelled or a terminal error.
func (q *Queue) Subscribe(ctx context.Context, handler queue.Handler) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		tasks, err := q.Pop(ctx, 1, 100*time.Millisecond)
		if errors.Is(err, queue.ErrEmpty) {
			continue
		}
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			continue
		}
		for _, t := range tasks {
			handlerErr := handler(ctx, t.Message)
			switch {
			case handlerErr == nil:
				_ = q.Ack(ctx, t.ID)
			case errors.Is(handlerErr, queue.ErrDrop):
				_ = q.Ack(ctx, t.ID)
			default:
				_ = q.Retry(ctx, t, handlerErr.Error())
			}
		}
	}
}

func (q *Queue) StreamLen() int64 {
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
