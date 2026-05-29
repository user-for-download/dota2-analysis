package inmem

import (
	"context"
	"sync/atomic"
)

type lease struct {
	url      string
	release  func(context.Context) error
	success  func(context.Context) error
	failure  func(context.Context, error) error
	released atomic.Bool
}

func (l *lease) URL() string {
	return l.url
}

func (l *lease) Release(ctx context.Context) error {
	if l == nil || l.release == nil {
		return nil
	}
	if !l.released.CompareAndSwap(false, true) {
		return nil
	}
	return l.release(ctx)
}

func (l *lease) MarkSuccess(ctx context.Context) {
	if l == nil || l.success == nil {
		return
	}
	_ = l.success(ctx)
}

func (l *lease) MarkFailure(ctx context.Context, err error) {
	if l == nil || l.failure == nil {
		return
	}
	_ = l.failure(ctx, err)
}
