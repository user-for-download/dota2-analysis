package lookupstore

import (
	"context"
	"time"
)

type PatchInfo struct {
	ID        int
	Name      string
	StartedAt time.Time
}

type LookupReader interface {
	HeroIDs(ctx context.Context) (map[int]struct{}, error)
	ItemIDs(ctx context.Context) (map[int]struct{}, error)
	PatchByTimestamp(ctx context.Context, unixSec int64) (PatchInfo, error)
}