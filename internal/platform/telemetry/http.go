package telemetry

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"

	platformmetrics "go-order-management-system/internal/platform/metrics"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const (
	TraceIDHeader = "X-Trace-ID"
	SpanIDHeader  = "X-Span-ID"
)

func InstrumentHTTP(service string, next http.Handler) http.Handler {
	if next == nil {
		next = http.NotFoundHandler()
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if excludedPath(request.URL.Path) {
			next.ServeHTTP(writer, request)
			return
		}

		parent := otel.GetTextMapPropagator().Extract(request.Context(), propagation.HeaderCarrier(request.Header))
		routeGroup := platformmetrics.RouteGroup(request.URL.Path)
		spanName := request.Method + " " + routeGroup
		ctx, span := Tracer().Start(
			parent,
			spanName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("http.request.method", boundedMethod(request.Method)),
				attribute.String("go_order.route_group", routeGroup),
			),
		)
		defer span.End()

		recorder := &statusRecorder{ResponseWriter: writer, status: http.StatusOK}
		if spanContext := span.SpanContext(); spanContext.IsValid() {
			recorder.Header().Set(TraceIDHeader, spanContext.TraceID().String())
			recorder.Header().Set(SpanIDHeader, spanContext.SpanID().String())
		}
		next.ServeHTTP(recorder, request.WithContext(ctx))

		span.SetAttributes(attribute.Int("http.response.status_code", recorder.status))
		if recorder.status >= http.StatusInternalServerError {
			span.SetStatus(codes.Error, "server_error")
		}
	})
}

func InstrumentTransport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &tracingRoundTripper{base: base}
}

type tracingRoundTripper struct {
	base http.RoundTripper
}

func (transport *tracingRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil {
		return transport.base.RoundTrip(request)
	}
	routeGroup := "unmatched"
	upstream := "unknown"
	if request.URL != nil {
		routeGroup = platformmetrics.RouteGroup(request.URL.Path)
		if request.URL.Host != "" {
			upstream = request.URL.Host
		}
	}
	ctx, span := Tracer().Start(
		request.Context(),
		request.Method+" "+routeGroup,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("http.request.method", boundedMethod(request.Method)),
			attribute.String("server.address", bounded(upstream, 120)),
			attribute.String("go_order.route_group", routeGroup),
		),
	)

	cloned := request.Clone(ctx)
	cloned.Header = request.Header.Clone()
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(cloned.Header))
	response, err := transport.base.RoundTrip(cloned)
	if err != nil {
		span.SetAttributes(attribute.String("go_order.outcome", "transport_error"))
		span.SetStatus(codes.Error, "transport_error")
		span.End()
		return response, err
	}
	if response == nil {
		span.SetAttributes(attribute.String("go_order.outcome", "missing_response"))
		span.SetStatus(codes.Error, "missing_response")
		span.End()
		return nil, nil
	}
	span.SetAttributes(attribute.Int("http.response.status_code", response.StatusCode))
	if response.StatusCode >= http.StatusInternalServerError {
		span.SetStatus(codes.Error, "server_error")
	}
	if response.Body == nil {
		span.End()
		return response, nil
	}
	response.Body = &spanBody{ReadCloser: response.Body, span: span}
	return response, nil
}

type spanBody struct {
	io.ReadCloser
	span trace.Span
	once sync.Once
}

func (body *spanBody) Read(data []byte) (int, error) {
	count, err := body.ReadCloser.Read(data)
	if err != nil {
		body.finish()
	}
	return count, err
}

func (body *spanBody) Close() error {
	err := body.ReadCloser.Close()
	body.finish()
	return err
}

func (body *spanBody) finish() {
	body.once.Do(body.span.End)
}

func SpanIDs(ctx context.Context) (traceID string, spanID string, ok bool) {
	spanContext := trace.SpanContextFromContext(ctx)
	if !spanContext.IsValid() {
		return "", "", false
	}
	return spanContext.TraceID().String(), spanContext.SpanID().String(), true
}

func excludedPath(path string) bool {
	switch path {
	case "/metrics", "/ping", "/live", "/readyz":
		return true
	default:
		return false
	}
}

func boundedMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions:
		return method
	default:
		return "OTHER"
	}
}

func bounded(value string, limit int) string {
	if value == "" {
		return "unknown"
	}
	if len(value) > limit {
		return value[:limit]
	}
	return value
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (recorder *statusRecorder) WriteHeader(status int) {
	if recorder.status != http.StatusOK || status == http.StatusOK {
		return
	}
	recorder.status = status
	recorder.ResponseWriter.WriteHeader(status)
}

func (recorder *statusRecorder) Write(data []byte) (int, error) {
	return recorder.ResponseWriter.Write(data)
}

func (recorder *statusRecorder) ReadFrom(reader io.Reader) (int64, error) {
	return io.Copy(recorder.ResponseWriter, reader)
}

func (recorder *statusRecorder) Flush() {
	_ = http.NewResponseController(recorder.ResponseWriter).Flush()
}

func (recorder *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := recorder.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("response writer does not support hijacking")
	}
	return hijacker.Hijack()
}

func (recorder *statusRecorder) Push(target string, options *http.PushOptions) error {
	pusher, ok := recorder.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}
	return pusher.Push(target, options)
}

func (recorder *statusRecorder) Unwrap() http.ResponseWriter {
	return recorder.ResponseWriter
}

func StatusClass(status int) string {
	if status <= 0 {
		return "none"
	}
	return strconv.Itoa(status/100) + "xx"
}
