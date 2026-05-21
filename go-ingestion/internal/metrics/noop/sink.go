package noop

import (
	"context"

	"github.com/user-for-download/go-dota2/internal/metrics"
)

type Sink struct{}

func New() Sink { return Sink{} }

var (
	_ metrics.Sink   = Sink{}
	_ metrics.Reader = Sink{}
)

func (Sink) IngestSuccess(context.Context)                                    {}
func (Sink) IngestFailure(context.Context, metrics.FailureKind, int64, error) {}
func (Sink) ParseSuccess(context.Context)                                      {}
func (Sink) ParseFailure(context.Context, metrics.FailureKind)                {}
func (Sink) ParseDuplicate(context.Context)                                     {}
func (Sink) FetchSuccess(context.Context)                                       {}
func (Sink) FetchFailure(context.Context, metrics.FailureKind)                {}

func (Sink) RecordQueueDepth(context.Context, string, int64, int64) {}

func (Sink) RecordLatency(context.Context, metrics.Stage, float64) {}

func (Sink) Snapshot(context.Context) (metrics.Snapshot, error) {
	return metrics.Snapshot{FailuresByKind: map[metrics.FailureKind]uint64{}}, nil
}
