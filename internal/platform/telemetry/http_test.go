package telemetry

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestInstrumentHTTPContinuesW3CTraceWithBoundedSpanName(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
		_ = provider.Shutdown(context.Background())
	})

	const traceID = "0123456789abcdef0123456789abcdef"
	handler := InstrumentHTTP("order-service", http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		currentTraceID, _, ok := SpanIDs(request.Context())
		if !ok || currentTraceID != traceID {
			t.Fatalf("expected continued trace %s, got %s", traceID, currentTraceID)
		}
		writer.WriteHeader(http.StatusCreated)
	}))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/orders/12345?token=secret", nil)
	request.Header.Set("traceparent", "00-"+traceID+"-0123456789abcdef-01")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)
	if response.Header().Get(TraceIDHeader) != traceID {
		t.Fatalf("unexpected response trace id: %q", response.Header().Get(TraceIDHeader))
	}
	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected one span, got %d", len(spans))
	}
	if spans[0].Name() != "GET api_orders" {
		t.Fatalf("unexpected span name: %q", spans[0].Name())
	}
	for _, attr := range spans[0].Attributes() {
		value := attr.Value.String()
		if strings.Contains(value, "12345") || strings.Contains(value, "secret") {
			t.Fatalf("unbounded request data leaked into span attribute: %s=%s", attr.Key, value)
		}
	}
}

func TestInstrumentHTTPBoundsUnknownMethodInSpanName(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previousProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		_ = provider.Shutdown(context.Background())
	})

	handler := InstrumentHTTP("order-service", http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest("CUSTOM-"+strings.Repeat("X", 128), "/api/v1/orders/12345", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)
	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected one span, got %d", len(spans))
	}
	if spans[0].Name() != "OTHER api_orders" {
		t.Fatalf("unknown HTTP method leaked into server span name: %q", spans[0].Name())
	}
}

func TestInstrumentTransportInjectsChildTraceContext(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
		_ = provider.Shutdown(context.Background())
	})

	ctx, parent := provider.Tracer("test").Start(context.Background(), "parent")
	captured := ""
	transport := InstrumentTransport(roundTripFunc(func(request *http.Request) (*http.Response, error) {
		captured = request.Header.Get("traceparent")
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("ok")),
			Request:    request,
		}, nil
	}))
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://inventory-service:8085/internal/v1/inventory/reservations/12345", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	_ = response.Body.Close()
	parent.End()

	if !strings.Contains(captured, parent.SpanContext().TraceID().String()) {
		t.Fatalf("outbound traceparent did not contain parent trace id: %q", captured)
	}
	spans := recorder.Ended()
	if len(spans) != 2 {
		t.Fatalf("expected parent and client spans, got %d", len(spans))
	}
	if spans[0].Name() != "POST internal_inventory" {
		t.Fatalf("unexpected client span name: %q", spans[0].Name())
	}
}

func TestInstrumentTransportBoundsUnknownMethodInSpanName(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previousProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		_ = provider.Shutdown(context.Background())
	})

	transport := InstrumentTransport(roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("ok")),
			Request:    request,
		}, nil
	}))
	request, err := http.NewRequest("CUSTOM-"+strings.Repeat("X", 128), "http://inventory-service:8085/internal/v1/inventory/reservations/12345", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	_ = response.Body.Close()

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected one client span, got %d", len(spans))
	}
	if spans[0].Name() != "OTHER internal_inventory" {
		t.Fatalf("unknown HTTP method leaked into client span name: %q", spans[0].Name())
	}
}

func TestTraceHandlerAddsCorrelationFields(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	ctx, span := provider.Tracer("test").Start(context.Background(), "log")
	defer span.End()

	var output bytes.Buffer
	logger := slog.New(NewTraceHandler(slog.NewJSONHandler(&output, nil)))
	logger.InfoContext(ctx, "correlated")
	text := output.String()
	if !strings.Contains(text, span.SpanContext().TraceID().String()) || !strings.Contains(text, span.SpanContext().SpanID().String()) {
		t.Fatalf("trace correlation fields missing from log: %s", text)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
