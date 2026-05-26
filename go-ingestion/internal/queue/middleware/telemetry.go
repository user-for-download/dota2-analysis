package middleware

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/queue"
)

// LatencySink abstracts metrics recording so this middleware doesn't depend
// on a specific metrics implementation.
type LatencySink interface {
	RecordLatency(ctx context.Context, stage string, durationMs float64)
}

// ──────────────────────────────────────────────
// Publisher Decorator
// ──────────────────────────────────────────────

type TracedPublisher struct {
	next       queue.Publisher
	propagator propagation.TextMapPropagator
}

func NewTracedPublisher(next queue.Publisher) *TracedPublisher {
	return &TracedPublisher{
		next:       next,
		propagator: otel.GetTextMapPropagator(),
	}
}

func (t *TracedPublisher) Publish(ctx context.Context, msg queue.Message) error {
	if msg.Headers == nil {
		msg.Headers = make(map[string]string)
	}

	// Inject trace context into the agnostic Headers map
	carrier := propagation.MapCarrier(msg.Headers)
	t.propagator.Inject(ctx, carrier)

	return t.next.Publish(ctx, msg)
}

// ──────────────────────────────────────────────
// Subscriber Decorator
// ──────────────────────────────────────────────

type TracedSubscriber struct {
	next       queue.Subscriber
	propagator propagation.TextMapPropagator
	tracer     trace.Tracer
	stage      string
	metrics    LatencySink
}

func NewTracedSubscriber(next queue.Subscriber, stage string, metrics LatencySink) *TracedSubscriber {
	return &TracedSubscriber{
		next:       next,
		propagator: otel.GetTextMapPropagator(),
		tracer:     otel.Tracer("queue.subscriber"),
		stage:      stage,
		metrics:    metrics,
	}
}

func (t *TracedSubscriber) Subscribe(ctx context.Context, handler queue.Handler) error {
	wrappedHandler := func(handlerCtx context.Context, msg queue.Message) error {
		// Extract trace context from the headers
		carrier := propagation.MapCarrier(msg.Headers)
		extractedCtx := t.propagator.Extract(handlerCtx, carrier)

		spanCtx, span := t.tracer.Start(extractedCtx, "subscriber.process",
			trace.WithAttributes(attribute.String("task.id", msg.ID)),
		)
		defer span.End()

		start := time.Now()
		err := handler(spanCtx, msg)

		if t.metrics != nil {
			t.metrics.RecordLatency(spanCtx, t.stage, float64(time.Since(start).Milliseconds()))
		}

		if err != nil && err != queue.ErrDrop {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else {
			span.SetStatus(codes.Ok, "success")
		}

		return err
	}

	return t.next.Subscribe(ctx, wrappedHandler)
}
