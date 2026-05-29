package matchstore

import (
	"context"
	"github.com/user-for-download/dota2-analysis/go-core/domain"
)

// MatchWriter persists a full match to the datastore.
type MatchWriter interface {
	IngestMatch(ctx context.Context, m domain.Match) error
}

// MatchRef is a match_id paired with its start_time (unix epoch), enabling
// partition-pruned queries against the time-partitioned matches table.
type MatchRef struct {
	MatchID   int64
	StartTime int64
}

// MatchReader queries match-level metadata.
type MatchReader interface {
	// UnknownIDs accepts candidate (match_id, start_time) pairs so the
	// partition-pruned query can filter by start_time and avoid scanning
	// every quarterly partition.  Returns match IDs that need processing
	// (not yet ingested or not yet parsed).
	UnknownIDs(ctx context.Context, candidates []MatchRef) ([]int64, error)
	Counts(ctx context.Context) (Counts, error)
	IsIngested(ctx context.Context, matchID int64, startTime int64) (bool, error)
}

// MatchStore combines reading and writing.
type MatchStore interface {
	MatchReader
	MatchWriter
}

// Counts holds aggregate match/player counts.
type Counts struct {
	Matches int64
	Players int64
}
