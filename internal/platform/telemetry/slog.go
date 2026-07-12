package telemetry

import (
	"context"
	"io"
	"log/slog"
)

func NewTraceHandler(next slog.Handler) slog.Handler {
	if next == nil {
		next = slog.NewJSONHandler(io.Discard, nil)
	}
	return &traceHandler{next: next}
}

type traceHandler struct {
	next slog.Handler
}

func (handler *traceHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return handler.next.Enabled(ctx, level)
}

func (handler *traceHandler) Handle(ctx context.Context, record slog.Record) error {
	if traceID, spanID, ok := SpanIDs(ctx); ok {
		record.AddAttrs(
			slog.String("trace_id", traceID),
			slog.String("span_id", spanID),
		)
	}
	return handler.next.Handle(ctx, record)
}

func (handler *traceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &traceHandler{next: handler.next.WithAttrs(attrs)}
}

func (handler *traceHandler) WithGroup(name string) slog.Handler {
	return &traceHandler{next: handler.next.WithGroup(name)}
}
