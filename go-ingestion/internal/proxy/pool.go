package proxy

import (
	"context"
	"sync/atomic"
	"time"
)

type Lease struct {
	URL      string
	release  func(context.Context) error
	success  func(context.Context) error
	failure  func(context.Context, error) error
	released atomic.Bool
}

func (l *Lease) Release(ctx context.Context) error {
	if l == nil || l.release == nil {
		return nil
	}
	if !l.released.CompareAndSwap(false, true) {
		return nil
	}
	return l.release(ctx)
}

func (l *Lease) MarkSuccess(ctx context.Context) {
	if l == nil || l.success == nil {
		return
	}
	_ = l.success(ctx)
}

func (l *Lease) MarkFailure(ctx context.Context, err error) {
	if l == nil || l.failure == nil {
		return
	}
	_ = l.failure(ctx, err)
}

func NewLease(
	url string,
	release func(context.Context) error,
	success func(context.Context) error,
	failure func(context.Context, error) error,
) *Lease {
	return &Lease{
		URL:     url,
		release: release,
		success: success,
		failure: failure,
	}
}

type Pool interface {
	Acquire(ctx context.Context, hold time.Duration) (*Lease, error)
	Size(ctx context.Context) (int, error)
	Replace(ctx context.Context, healthy []string) error
	Add(ctx context.Context, healthy []string) error
}
