package bootstrap

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// NewLogger wraps a slog.Handler with OTel trace ID injection.
// Every log record produced through the returned logger will include
// trace_id and span_id attributes when a span is active in the context.
func NewLogger(h slog.Handler) *slog.Logger {
	return slog.New(otelHandler{Handler: h})
}

type otelHandler struct {
	slog.Handler
}

func (h otelHandler) Handle(ctx context.Context, r slog.Record) error {
	if spanCtx := trace.SpanContextFromContext(ctx); spanCtx.IsValid() {
		r.AddAttrs(
			slog.String("trace_id", spanCtx.TraceID().String()),
			slog.String("span_id", spanCtx.SpanID().String()),
		)
	}
	return h.Handler.Handle(ctx, r)
}
