package queue

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Middleware is a function that wraps a Handler.
type Middleware func(Handler) Handler

// Chain applies middlewares to a handler.
func Chain(h Handler, mws ...Middleware) Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// ErrorTranslator translates common Go errors into queue.ErrDrop
// so workers don't need to know about queue semantics.
func ErrorTranslator() Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, msg Message) error {
			err := next(ctx, msg)
			if err == nil {
				return nil
			}

			// If it's already ErrDrop, pass it through
			if errors.Is(err, ErrDrop) {
				return err
			}

			// Translate JSON syntax errors to ErrDrop (permanent failure)
			var syntaxErr *json.SyntaxError
			var unmarshalErr *json.UnmarshalTypeError
			if errors.As(err, &syntaxErr) || errors.As(err, &unmarshalErr) {
				return ErrDrop
			}

			return err
		}
	}
}

// TTLExtender automatically extends the TTL of a payload while the worker is running.
// This removes the need for workers to manually call store.ExtendTTL.
func TTLExtender(extend func(ctx context.Context) error, interval time.Duration) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, msg Message) error {
			if extend == nil {
				return next(ctx, msg)
			}

			done := make(chan struct{})
			defer close(done)

			go func() {
				ticker := time.NewTicker(interval)
				defer ticker.Stop()

				for {
					select {
					case <-done:
						return
					case <-ctx.Done():
						return
					case <-ticker.C:
						_ = extend(ctx)
					}
				}
			}()

			return next(ctx, msg)
		}
	}
}
