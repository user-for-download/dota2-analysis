package queue

import (
	"context"
	"errors"
	"math/rand"
	"time"
)

// ──────────────────────────────────────────────
// Task — internal queue message with lifecycle state
// ──────────────────────────────────────────────

type Task struct {
	ID         string
	Payload    []byte
	RetryCount int
	Ctx        context.Context
}

var ErrEmpty = errors.New("queue: no tasks available")

// ──────────────────────────────────────────────
// Legacy Queue interface (7 methods) — still supported for
// backward compatibility. Prefer Publisher / Subscriber below.
// ──────────────────────────────────────────────

type Queue interface {
	Push(ctx context.Context, payload []byte) error
	Pop(ctx context.Context, batch int, block time.Duration) ([]Task, error)
	Ack(ctx context.Context, taskID string) error
	Retry(ctx context.Context, t Task, reason string) error
	RecoverStale(ctx context.Context, idleFor time.Duration, max int) ([]Task, error)
	StreamLen() int64
	InFlightLen() int64
}

type QueueStats struct {
	StreamLen int64
	InFlight  int64
}

type QueueObservable interface {
	Queue
	Stats(ctx context.Context) (QueueStats, error)
}

// ──────────────────────────────────────────────
// Publisher / Subscriber / PubSub — minimal interfaces per Interface Segregation
// ──────────────────────────────────────────────

// Publisher enqueues a payload for asynchronous processing.
type Publisher interface {
	Publish(ctx context.Context, payload []byte) error
}

// PubSub combines Publisher and Subscriber for components that need both roles.
type PubSub interface {
	Publisher
	Subscriber
}

// Message is delivered to a Handler. It carries only the data the
// handler needs — no retry-count or internal lifecycle state.
type Message struct {
	ID      string
	Payload []byte
}

// ErrDrop signals that a message should be acknowledged and dropped
// (not retried). Return it from a Handler for permanent failures.
var ErrDrop = errors.New("queue: drop message")

// Handler processes a single Message.  Return nil to ACK, ErrDrop to
// acknowledge without retry, or any other error to retry (with backoff
// and eventual DLQ).
type Handler func(ctx context.Context, msg Message) error

// Subscriber receives messages and manages the full lifecycle:
// polling, acknowledgement, retries, backpressure, stale recovery,
// and dead-letter queuing.
type Subscriber interface {
	Subscribe(ctx context.Context, handler Handler) error
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