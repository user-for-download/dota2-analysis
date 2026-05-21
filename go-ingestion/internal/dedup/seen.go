package dedup

import "context"

type Seen interface {
	MarkSeen(ctx context.Context, key string) (alreadySeen bool, err error)
	IsSeen(ctx context.Context, key string) (bool, error)
	CheckBatch(ctx context.Context, keys []string) ([]bool, error)
}
