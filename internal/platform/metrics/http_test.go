package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInstrumentHTTPUsesBoundedRouteAndStatusLabels(t *testing.T) {
	previous := Default
	Default = NewRegistry()
	t.Cleanup(func() { Default = previous })

	handler := InstrumentHTTP("order-service", http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusConflict)
		_, _ = writer.Write([]byte("conflict"))
	}))

	request := httptest.NewRequest(http.MethodPost, "/api/v1/orders/123/pay", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	body := string(Default.Gather())
	for _, expected := range []string{
		`go_order_http_server_requests_total{method="POST",route_group="api_orders",service="order-service",status_class="4xx"} 1`,
		`go_order_http_server_response_bytes_total{method="POST",route_group="api_orders",service="order-service",status_class="4xx"} 8`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected %q in metrics:\n%s", expected, body)
		}
	}
	if strings.Contains(body, "123") {
		t.Fatalf("raw resource identifier must not appear in metric labels:\n%s", body)
	}
}

func TestInstrumentHTTPServesMetricsWithoutInstrumentingScrape(t *testing.T) {
	previous := Default
	Default = NewRegistry()
	t.Cleanup(func() { Default = previous })
	Default.SetGauge("test_ready", "Test readiness.", nil, 1)

	called := false
	handler := InstrumentHTTP("identity-service", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if called {
		t.Fatal("application handler must not receive the metrics scrape")
	}
	if contentType := response.Header().Get("Content-Type"); !strings.Contains(contentType, "version=0.0.4") {
		t.Fatalf("unexpected content type %q", contentType)
	}
	if !strings.Contains(response.Body.String(), "test_ready 1") {
		t.Fatalf("unexpected metrics body:\n%s", response.Body.String())
	}
	if strings.Contains(response.Body.String(), "go_order_http_server_requests_total") {
		t.Fatalf("metrics scrapes must not recursively instrument themselves:\n%s", response.Body.String())
	}
}

func TestRouteGroupDoesNotExposeRawPaths(t *testing.T) {
	cases := map[string]string{
		"/api/v1/users/42":                      "api_users",
		"/internal/v1/orders/99/cancel-timeout": "internal_orders",
		"/something/unbounded/123":              "unmatched",
	}
	for path, expected := range cases {
		if actual := RouteGroup(path); actual != expected {
			t.Fatalf("RouteGroup(%q) = %q, want %q", path, actual, expected)
		}
	}
}
