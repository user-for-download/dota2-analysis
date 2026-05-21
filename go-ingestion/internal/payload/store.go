package payload

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("payload: not found")

type Store interface {
	Put(ctx context.Context, key string, payload []byte, ttl time.Duration) error
	Get(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
	ExtendTTL(ctx context.Context, key string, ttl time.Duration) error
}
