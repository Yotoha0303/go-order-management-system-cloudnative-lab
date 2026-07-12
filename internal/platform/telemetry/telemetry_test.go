package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"go.opentelemetry.io/otel"
)

var (
	traceIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
	spanIDPattern  = regexp.MustCompile(`^[0-9a-f]{16}$`)
)

func TestSetupCreatesHTTPTraceContextWithoutExporter(t *testing.T) {
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

	handler := InstrumentHTTP("test-service", http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if _, _, ok := SpanIDs(request.Context()); !ok {
			t.Fatal("expected handler context to contain a valid span")
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/orders", nil))

	traceID := response.Header().Get(TraceIDHeader)
	spanID := response.Header().Get(SpanIDHeader)
	if !traceIDPattern.MatchString(traceID) || !spanIDPattern.MatchString(spanID) {
		t.Fatalf("unexpected trace response headers: trace_id=%q span_id=%q", traceID, spanID)
	}
}
