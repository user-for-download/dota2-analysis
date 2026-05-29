package proxy

import (
	"context"
	"time"
)

// Lease represents an acquired proxy that must be released when done.
type Lease interface {
	// URL returns the proxy URL.
	URL() string
	// MarkSuccess records that the proxy successfully handled a request.
	MarkSuccess(ctx context.Context)
	// MarkFailure records that the proxy failed to handle a request.
	MarkFailure(ctx context.Context, err error)
	// Release returns the proxy to the pool. It must be called exactly once.
	Release(ctx context.Context) error
}

// Pool provides access to a rotating set of proxies.
type Pool interface {
	// Acquire blocks until a proxy is available or the context is cancelled.
	Acquire(ctx context.Context, hold time.Duration) (Lease, error)
	// Size returns the number of proxies currently in the pool.
	Size(ctx context.Context) (int, error)
	// Replace completely replaces the current set of proxies.
	Replace(ctx context.Context, healthy []string) error
	// Add adds new proxies to the pool.
	Add(ctx context.Context, healthy []string) error
}
