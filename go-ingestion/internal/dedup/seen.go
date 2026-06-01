package dedup

import "context"

type Seen interface {
	MarkSeen(ctx context.Context, key string) (alreadySeen bool, err error)
	IsSeen(ctx context.Context, key string) (bool, error)
	// CheckBatch checks multiple keys in one network round-trip.
	// Returns a map where true = already seen, false = new.
	CheckBatch(ctx context.Context, keys []string) (map[string]bool, error)
}
