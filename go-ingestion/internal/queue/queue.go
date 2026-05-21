package queue

import (
	"context"
	"errors"
	"math/rand"
	"time"
)

type Task struct {
	ID         string
	Payload    []byte
	RetryCount int
	Ctx        context.Context
}

var ErrEmpty = errors.New("queue: no tasks available")

type Queue interface {
	Push(ctx context.Context, payload []byte) error
	Pop(ctx context.Context, batch int, block time.Duration) ([]Task, error)
	Ack(ctx context.Context, taskID string) error
	Retry(ctx context.Context, t Task, reason string) error
	RecoverStale(ctx context.Context, idleFor time.Duration, max int) ([]Task, error)
	PendingLen() int64
	InFlightLen() int64
}

type QueueStats struct {
	Pending   int64
	InFlight int64
}

type QueueObservable interface {
	Queue
	Stats(ctx context.Context) (QueueStats, error)
}

type RetryPolicy struct {
	MaxRetries int
	MaxBackoff time.Duration
}

func (p RetryPolicy) Backoff(retryCount int) time.Duration {
	if retryCount <= 0 {
		return 0
	}
	d := time.Duration(retryCount*retryCount) * time.Second
	if p.MaxBackoff > 0 && d > p.MaxBackoff {
		d = p.MaxBackoff
	}
	jitter := float64(d) * 0.25 * (2*rand.Float64() - 1)
	d = time.Duration(float64(d) + jitter)
	if d < 0 {
		return 0
	}
	return d
}

func (p RetryPolicy) ShouldDLQ(retryCount int) bool {
	return p.MaxRetries > 0 && retryCount > p.MaxRetries
}