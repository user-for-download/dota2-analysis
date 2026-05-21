package otelmetrics

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/user-for-download/go-dota2/internal/metrics"
)

type QueueStatsProvider interface {
	QueueDepth(ctx context.Context, stream string) (pending int64, inFlight int64)
}

type Sink struct {
	ingestSuccess  metric.Int64Counter
	ingestFailure  metric.Int64Counter
	parseSuccess   metric.Int64Counter
	parseFailure   metric.Int64Counter
	parseDuplicate metric.Int64Counter
	fetchSuccess   metric.Int64Counter
	fetchFailure   metric.Int64Counter
	stageLatency   metric.Float64Histogram
}

func New() (*Sink, error) {
	return NewWithProviders(nil, nil)
}

func NewWithProviders(fetchStatsProvider, parseStatsProvider QueueStatsProvider) (*Sink, error) {
	meter := otel.Meter("go-dota2")

	is, err := meter.Int64Counter("dota2.ingest.success",
		metric.WithDescription("Number of successfully ingested matches"))
	if err != nil {
		return nil, err
	}

	ifail, err := meter.Int64Counter("dota2.ingest.failure",
		metric.WithDescription("Number of failed ingests"))
	if err != nil {
		return nil, err
	}

	ps, err := meter.Int64Counter("dota2.parse.success",
		metric.WithDescription("Number of successfully parsed matches"))
	if err != nil {
		return nil, err
	}

	pfail, err := meter.Int64Counter("dota2.parse.failure",
		metric.WithDescription("Number of failed parses"))
	if err != nil {
		return nil, err
	}

	pdup, err := meter.Int64Counter("dota2.parse.duplicate",
		metric.WithDescription("Number of duplicate parses (matches already in database)"))
	if err != nil {
		return nil, err
	}

	fs, err := meter.Int64Counter("dota2.fetch.success",
		metric.WithDescription("Number of successfully fetched matches"))
	if err != nil {
		return nil, err
	}

	ffail, err := meter.Int64Counter("dota2.fetch.failure",
		metric.WithDescription("Number of failed fetches"))
	if err != nil {
		return nil, err
	}

	slatency, err := meter.Float64Histogram("dota2.stage.latency",
		metric.WithDescription("Processing latency per stage in milliseconds"),
		metric.WithUnit("ms"))
	if err != nil {
		return nil, err
	}

	sink := &Sink{
		ingestSuccess:  is,
		ingestFailure:  ifail,
		parseSuccess:   ps,
		parseFailure:   pfail,
		parseDuplicate: pdup,
		fetchSuccess:   fs,
		fetchFailure:   ffail,
		stageLatency:   slatency,
	}

	if fetchStatsProvider != nil {
		_, err := meter.Int64ObservableGauge("dota2.fetch.queue.pending",
			metric.WithInt64Callback(func(ctx context.Context, o metric.Int64Observer) error {
				_, inFlight := fetchStatsProvider.QueueDepth(ctx, "fetch")
				o.Observe(inFlight)
				return nil
			}),
		)
		if err != nil {
			return nil, fmt.Errorf("queue pending gauge: %w", err)
		}
	}

	if parseStatsProvider != nil {
		_, err := meter.Int64ObservableGauge("dota2.parse.queue.in_flight",
			metric.WithInt64Callback(func(ctx context.Context, o metric.Int64Observer) error {
				pending, _ := parseStatsProvider.QueueDepth(ctx, "parse")
				o.Observe(pending)
				return nil
			}),
		)
		if err != nil {
			return nil, fmt.Errorf("queue in_flight gauge: %w", err)
		}
	}

	return sink, nil
}

func (s *Sink) IngestSuccess(ctx context.Context) {
	s.ingestSuccess.Add(ctx, 1)
}

func (s *Sink) IngestFailure(ctx context.Context, kind metrics.FailureKind, matchID int64, err error) {
	s.ingestFailure.Add(ctx, 1, metric.WithAttributes(
		attribute.String("kind", kind.String())))
}

func (s *Sink) ParseSuccess(ctx context.Context) {
	s.parseSuccess.Add(ctx, 1)
}

func (s *Sink) ParseFailure(ctx context.Context, kind metrics.FailureKind) {
	s.parseFailure.Add(ctx, 1, metric.WithAttributes(
		attribute.String("kind", kind.String())))
}

func (s *Sink) ParseDuplicate(ctx context.Context) {
	s.parseDuplicate.Add(ctx, 1)
}

func (s *Sink) FetchSuccess(ctx context.Context) {
	s.fetchSuccess.Add(ctx, 1)
}

func (s *Sink) FetchFailure(ctx context.Context, kind metrics.FailureKind) {
	s.fetchFailure.Add(ctx, 1, metric.WithAttributes(
		attribute.String("kind", kind.String())))
}

func (s *Sink) RecordQueueDepth(ctx context.Context, stream string, pending, inFlight int64) {}

func (s *Sink) RecordLatency(ctx context.Context, stage metrics.Stage, durationMs float64) {
	s.stageLatency.Record(ctx, durationMs, metric.WithAttributes(
		attribute.String("stage", string(stage))))
}