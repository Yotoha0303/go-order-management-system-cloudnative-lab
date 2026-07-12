package metrics

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var durationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

func InstrumentHTTP(service string, next http.Handler, collectors ...Collector) http.Handler {
	if next == nil {
		next = http.NotFoundHandler()
	}
	metricsHandler := Default.Handler(collectors...)
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/metrics" {
			metricsHandler.ServeHTTP(writer, request)
			return
		}

		started := time.Now()
		recorder := &responseRecorder{ResponseWriter: writer, status: http.StatusOK}
		next.ServeHTTP(recorder, request)
		labels := Labels{
			"service":      service,
			"method":       boundedMethod(request.Method),
			"route_group":  RouteGroup(request.URL.Path),
			"status_class": StatusClass(recorder.status),
		}
		Default.IncCounter(
			"go_order_http_server_requests_total",
			"Total HTTP server requests by service, method, route group and status class.",
			labels,
		)
		Default.AddCounter(
			"go_order_http_server_response_bytes_total",
			"Total HTTP response bytes by service, method, route group and status class.",
			labels,
			float64(recorder.bytes),
		)
		Default.ObserveHistogram(
			"go_order_http_server_request_duration_seconds",
			"HTTP server request duration in seconds by service, method, route group and status class.",
			labels,
			time.Since(started).Seconds(),
			durationBuckets,
		)
	})
}

func RecordHTTPClientAttempt(upstream, operation, outcome string, status int, retryable bool, duration time.Duration) {
	labels := Labels{
		"upstream":     boundedValue(upstream),
		"operation":    boundedValue(operation),
		"outcome":      boundedOutcome(outcome),
		"status_class": StatusClass(status),
		"retryable":    strconv.FormatBool(retryable),
	}
	Default.IncCounter(
		"go_order_http_client_attempts_total",
		"Total upstream HTTP attempts by upstream, operation, outcome, status class and retryability.",
		labels,
	)
	Default.ObserveHistogram(
		"go_order_http_client_attempt_duration_seconds",
		"Upstream HTTP attempt duration in seconds by upstream, operation, outcome, status class and retryability.",
		labels,
		duration.Seconds(),
		durationBuckets,
	)
}

func RecordCircuitRejection(upstream, operation string) {
	Default.IncCounter(
		"go_order_http_client_circuit_rejections_total",
		"Total upstream HTTP requests rejected before network I/O because a circuit was open.",
		Labels{"upstream": boundedValue(upstream), "operation": boundedValue(operation)},
	)
}

func StatusClass(status int) string {
	if status <= 0 {
		return "none"
	}
	class := status / 100
	if class < 1 || class > 5 {
		return "other"
	}
	return strconv.Itoa(class) + "xx"
}

func RouteGroup(path string) string {
	clean := strings.TrimSpace(path)
	switch clean {
	case "/ping":
		return "ping"
	case "/live":
		return "live"
	case "/readyz":
		return "readyz"
	case "/metrics":
		return "metrics"
	}

	groups := []struct {
		prefix string
		name   string
	}{
		{"/api/v1/auth", "api_auth"},
		{"/api/v1/users", "api_users"},
		{"/api/v1/products", "api_products"},
		{"/api/v1/inventory", "api_inventory"},
		{"/api/v1/stock-logs", "api_stock_logs"},
		{"/api/v1/orders", "api_orders"},
		{"/internal/v1/operations", "internal_operations"},
		{"/internal/v1/users", "internal_users"},
		{"/internal/v1/products", "internal_products"},
		{"/internal/v1/inventory", "internal_inventory"},
		{"/internal/v1/orders", "internal_orders"},
		{"/internal/v1", "internal_other"},
	}
	for _, group := range groups {
		if clean == group.prefix || strings.HasPrefix(clean, group.prefix+"/") {
			return group.name
		}
	}
	return "unmatched"
}

func boundedMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions:
		return method
	default:
		return "OTHER"
	}
}

func boundedOutcome(outcome string) string {
	switch outcome {
	case "success", "transport_error", "http_error", "circuit_open":
		return outcome
	default:
		return "other"
	}
}

func boundedValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	if len(value) > 80 {
		return value[:80]
	}
	return value
}

type responseRecorder struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (recorder *responseRecorder) WriteHeader(status int) {
	if recorder.status != http.StatusOK || status == http.StatusOK {
		return
	}
	recorder.status = status
	recorder.ResponseWriter.WriteHeader(status)
}

func (recorder *responseRecorder) Write(data []byte) (int, error) {
	written, err := recorder.ResponseWriter.Write(data)
	recorder.bytes += int64(written)
	return written, err
}

func (recorder *responseRecorder) ReadFrom(reader io.Reader) (int64, error) {
	written, err := io.Copy(recorder.ResponseWriter, reader)
	recorder.bytes += written
	return written, err
}

func (recorder *responseRecorder) Flush() {
	_ = http.NewResponseController(recorder.ResponseWriter).Flush()
}

func (recorder *responseRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := recorder.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("response writer does not support hijacking")
	}
	return hijacker.Hijack()
}

func (recorder *responseRecorder) Push(target string, options *http.PushOptions) error {
	pusher, ok := recorder.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}
	return pusher.Push(target, options)
}

func (recorder *responseRecorder) Unwrap() http.ResponseWriter {
	return recorder.ResponseWriter
}
