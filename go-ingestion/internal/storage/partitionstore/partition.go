package partitionstore

import (
	"context"
	"time"
)

type PartitionAdmin interface {
	EnsurePartitions(ctx context.Context, until time.Time) error
}