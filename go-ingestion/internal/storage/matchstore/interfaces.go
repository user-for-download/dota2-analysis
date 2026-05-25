package matchstore

import "context"

// MatchWriter persists a full match to the datastore.
type MatchWriter interface {
	IngestMatch(ctx context.Context, m Match) error
}

// MatchReader queries match-level metadata.
type MatchReader interface {
	UnknownIDs(ctx context.Context, candidates []int64) ([]int64, error)
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
