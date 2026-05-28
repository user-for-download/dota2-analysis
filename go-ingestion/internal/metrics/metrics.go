package metrics

import (
	"context"
	"time"
)

type Sink interface {
	IngestSuccess(ctx context.Context)
	IngestFailure(ctx context.Context, kind FailureKind, matchID int64, err error)
	ParseSuccess(ctx context.Context)
	ParseFailure(ctx context.Context, kind FailureKind)
	ParseDuplicate(ctx context.Context)
	FetchSuccess(ctx context.Context)
	FetchFailure(ctx context.Context, kind FailureKind)
	RecordQueueDepth(ctx context.Context, stream string, pending, inFlight int64)
	RecordLatency(ctx context.Context, stage Stage, durationMs float64)
}

type Stage string

const (
	StageIngest Stage = "ingest"
	StageParse  Stage = "parse"
	StageFetch  Stage = "fetch"
	StageDiscover Stage = "discover"
	StageEnrich Stage = "enrich"
)

type Reader interface {
	Snapshot(ctx context.Context) (Snapshot, error)
}

type Snapshot struct {
	TakenAt         time.Time
	IngestSuccess   uint64
	IngestFailure   uint64
	ParseSuccess    uint64
	ParseFailure    uint64
	ParseDuplicate  uint64
	FetchSuccess    uint64
	FetchFailure    uint64
	FailuresByKind  map[FailureKind]uint64
	RecentFailures  []FailureEvent
}

type FailureEvent struct {
	At      time.Time   `json:"at"`
	Stage   string      `json:"stage"`
	Kind    FailureKind `json:"kind"`
	MatchID int64       `json:"match_id,omitempty"`
	Message string      `json:"message,omitempty"`
}

type FailureKind string

const (
	KindUnknown   FailureKind = "unknown"
	KindDecode    FailureKind = "decode"
	KindValidate  FailureKind = "validate"
	KindIngest    FailureKind = "ingest"
	KindDB        FailureKind = "db"
	KindHTTP      FailureKind = "http"
	KindTimeout   FailureKind = "timeout"
	KindRateLimit FailureKind = "rate_limit"
	KindNotFound  FailureKind = "not_found"
	KindProxy     FailureKind = "proxy"
	KindPayload   FailureKind = "payload"
	KindDraftSchema FailureKind = "draft_schema"
)

func (k FailureKind) String() string { return string(k) }

func AllKinds() []FailureKind {
	return []FailureKind{
		KindUnknown,
		KindDecode,
		KindValidate,
		KindIngest,
		KindDB,
		KindHTTP,
		KindTimeout,
		KindRateLimit,
		KindNotFound,
		KindProxy,
		KindPayload,
		KindDraftSchema,
	}
}
