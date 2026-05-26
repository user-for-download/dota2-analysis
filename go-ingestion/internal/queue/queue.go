package queue

import (
	"context"
	"errors"
	"math/rand"
	"time"
)

var (
	ErrEmpty = errors.New("queue: no tasks available")
	ErrDrop  = errors.New("queue: drop message")
)

// ──────────────────────────────────────────────
// Primitives
// ──────────────────────────────────────────────

// Message is the standard primitive for all queue operations.
// Headers allow transport-agnostic metadata (like tracing) without coupling.
type Message struct {
	ID      string
	Payload []byte
	Headers map[string]string
}

// Task represents an internal queue item with lifecycle state.
// Used by implementations, but hidden from the Subscriber handler.
type Task struct {
	Message
	RetryCount int
}

// ──────────────────────────────────────────────
// Interfaces (Black Box Boundaries)
// ──────────────────────────────────────────────

type Publisher interface {
	Publish(ctx context.Context, msg Message) error
}

type Handler func(ctx context.Context, msg Message) error

type Subscriber interface {
	Subscribe(ctx context.Context, handler Handler) error
}

type PubSub interface {
	Publisher
	Subscriber
}

// ──────────────────────────────────────────────
// Retry Logic
// ──────────────────────────────────────────────

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
