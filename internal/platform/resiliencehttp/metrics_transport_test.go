package resiliencehttp

import (
	"io"
	"net/http"
	"strings"
	"testing"

	platformmetrics "go-order-management-system/internal/platform/metrics"
)

type metricsRoundTripFunc func(*http.Request) (*http.Response, error)

func (function metricsRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestObserveTransportRecordsBoundedAttemptMetrics(t *testing.T) {
	previous := platformmetrics.Default
	platformmetrics.Default = platformmetrics.NewRegistry()
	t.Cleanup(func() { platformmetrics.Default = previous })

	transport := ObserveTransport(metricsRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Body:       io.NopCloser(strings.NewReader("unavailable")),
			Request:    request,
		}, nil
	}))
	request, err := http.NewRequest(http.MethodPost, "http://inventory-service:8085/internal/v1/inventory/reservations/order-123", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	_ = response.Body.Close()

	body := string(platformmetrics.Default.Gather())
	if !strings.Contains(body, `go_order_http_client_attempts_total{operation="internal_inventory",outcome="http_error",retryable="true",status_class="5xx",upstream="inventory-service:8085"} 1`) {
		t.Fatalf("unexpected client metrics:\n%s", body)
	}
	if strings.Contains(body, "order-123") {
		t.Fatalf("raw resource identifier must not be present in client metric labels:\n%s", body)
	}
}
