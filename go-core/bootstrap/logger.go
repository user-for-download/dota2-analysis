package bootstrap

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"go.opentelemetry.io/otel/trace"
)

// NewLogger wraps a slog.Handler with OTel trace ID injection.
// Every log record produced through the returned logger will include
// trace_id and span_id attributes when a span is active in the context.
func NewLogger(h slog.Handler) *slog.Logger {
	return slog.New(otelHandler{Handler: h})
}

// NewLoggerFromEnv creates a JSON logger that respects the LOG_LEVEL env var.
// Defaults to INFO if LOG_LEVEL is unset or invalid.
func NewLoggerFromEnv() *slog.Logger {
	level := slog.LevelInfo
	if envLevel := os.Getenv("LOG_LEVEL"); envLevel != "" {
		switch strings.ToLower(envLevel) {
		case "debug":
			level = slog.LevelDebug
		case "info":
			level = slog.LevelInfo
		case "warn", "warning":
			level = slog.LevelWarn
		case "error":
			level = slog.LevelError
		}
	}
	opts := &slog.HandlerOptions{Level: level}
	h := slog.NewJSONHandler(os.Stdout, opts)
	return NewLogger(h)
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
