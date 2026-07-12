package telemetry

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
)

func TestSetupCreatesValidLocalTraceWithoutExporter(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	t.Setenv("OTEL_TRACES_SAMPLER_ARG", "1")

	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	shutdown, err := Setup(context.Background(), "test-service")
	if err != nil {
		t.Fatalf("setup telemetry: %v", err)
	}
	t.Cleanup(func() {
		_ = shutdown(context.Background())
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
	})

	ctx, span := Tracer().Start(context.Background(), "local-span")
	defer span.End()
	traceID, spanID, ok := SpanIDs(ctx)
	if !ok {
		t.Fatal("expected a valid local span context")
	}
	if len(traceID) != 32 || len(spanID) != 16 {
		t.Fatalf("unexpected trace identifiers: trace_id=%q span_id=%q", traceID, spanID)
	}
}
