package inmem

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/user-for-download/go-dota2/internal/metrics"
)

type Sink struct {
	mu              sync.Mutex
	ingestSuccess   uint64
	ingestFailure   uint64
	parseSuccess    uint64
	parseFailure    uint64
	parseDuplicate  uint64
	fetchSuccess    uint64
	fetchFailure    uint64
	failuresByKind  map[metrics.FailureKind]uint64
	recent          []metrics.FailureEvent
}

func New() *Sink {
	return &Sink{
		failuresByKind: make(map[metrics.FailureKind]uint64),
	}
}

var _ metrics.Sink = (*Sink)(nil)
var _ metrics.Reader = (*Sink)(nil)

func (s *Sink) IngestSuccess(_ context.Context) {
	atomic.AddUint64(&s.ingestSuccess, 1)
}

func (s *Sink) IngestFailure(_ context.Context, k metrics.FailureKind, id int64, err error) {
	atomic.AddUint64(&s.ingestFailure, 1)
	s.mu.Lock()
	s.failuresByKind[k]++
	s.mu.Unlock()
	s.appendRecent("ingest", k, id, err)
}

func (s *Sink) ParseSuccess(_ context.Context) {
	atomic.AddUint64(&s.parseSuccess, 1)
}

func (s *Sink) ParseFailure(_ context.Context, k metrics.FailureKind) {
	atomic.AddUint64(&s.parseFailure, 1)
	s.mu.Lock()
	s.failuresByKind[k]++
	s.mu.Unlock()
}

func (s *Sink) ParseDuplicate(_ context.Context) {
	atomic.AddUint64(&s.parseDuplicate, 1)
}

func (s *Sink) FetchSuccess(_ context.Context) {
	atomic.AddUint64(&s.fetchSuccess, 1)
}

func (s *Sink) FetchFailure(_ context.Context, k metrics.FailureKind) {
	atomic.AddUint64(&s.fetchFailure, 1)
	s.mu.Lock()
	s.failuresByKind[k]++
	s.mu.Unlock()
}

func (s *Sink) RecordQueueDepth(_ context.Context, _ string, _, _ int64) {}

func (s *Sink) RecordLatency(_ context.Context, _ metrics.Stage, _ float64) {}

func (s *Sink) Snapshot(_ context.Context) (metrics.Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	byKind := make(map[metrics.FailureKind]uint64, len(s.failuresByKind))
	for k, v := range s.failuresByKind {
		byKind[k] = v
	}
	recent := make([]metrics.FailureEvent, len(s.recent))
	copy(recent, s.recent)
	return metrics.Snapshot{
		TakenAt:        time.Now(),
		IngestSuccess:  atomic.LoadUint64(&s.ingestSuccess),
		IngestFailure:  atomic.LoadUint64(&s.ingestFailure),
		ParseSuccess:   atomic.LoadUint64(&s.parseSuccess),
		ParseFailure:   atomic.LoadUint64(&s.parseFailure),
		ParseDuplicate: atomic.LoadUint64(&s.parseDuplicate),
		FetchSuccess:   atomic.LoadUint64(&s.fetchSuccess),
		FetchFailure:   atomic.LoadUint64(&s.fetchFailure),
		FailuresByKind: byKind,
		RecentFailures: recent,
	}, nil
}

func (s *Sink) appendRecent(stage string, kind metrics.FailureKind, id int64, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ev := metrics.FailureEvent{
		At:      time.Now(),
		Stage:   stage,
		Kind:    kind,
		MatchID: id,
	}
	if err != nil {
		ev.Message = err.Error()
	}
	const maxRecent = 100
	if len(s.recent) >= maxRecent {
		s.recent = s.recent[1:]
	}
	s.recent = append(s.recent, ev)
}