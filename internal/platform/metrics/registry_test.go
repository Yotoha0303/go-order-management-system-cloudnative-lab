package metrics

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestRegistryGatherCounterGaugeAndHistogram(t *testing.T) {
	registry := NewRegistry()
	registry.AddCounter("test_requests_total", "Test requests.", Labels{"method": "GET"}, 2)
	registry.SetGauge("test_workers", "Test workers.", Labels{"kind": "timeout"}, 3)
	registry.ObserveHistogram("test_latency_seconds", "Test latency.", Labels{"operation": "create"}, 0.2, []float64{0.1, 0.5})

	body := string(registry.Gather())
	for _, expected := range []string{
		"# TYPE test_requests_total counter",
		`test_requests_total{method="GET"} 2`,
		"# TYPE test_workers gauge",
		`test_workers{kind="timeout"} 3`,
		`test_latency_seconds_bucket{le="0.1",operation="create"} 0`,
		`test_latency_seconds_bucket{le="0.5",operation="create"} 1`,
		`test_latency_seconds_bucket{le="+Inf",operation="create"} 1`,
		`test_latency_seconds_count{operation="create"} 1`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected exposition to contain %q, got:\n%s", expected, body)
		}
	}
}

func TestRegistryConcurrentCounterUpdates(t *testing.T) {
	registry := NewRegistry()
	var wait sync.WaitGroup
	for worker := 0; worker < 20; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for index := 0; index < 100; index++ {
				registry.IncCounter("test_concurrent_total", "Concurrent counter.", Labels{"worker_type": "test"})
			}
		}()
	}
	wait.Wait()

	if body := string(registry.Gather()); !strings.Contains(body, `test_concurrent_total{worker_type="test"} 2000`) {
		t.Fatalf("unexpected concurrent counter exposition:\n%s", body)
	}
}

func TestHandlerRecordsCollectorErrorAndStillReturnsMetrics(t *testing.T) {
	registry := NewRegistry()
	handler := registry.Handler(Collector{
		Name: "database",
		Collect: func(context.Context, *Registry) error {
			return errors.New("database unavailable")
		},
	})

	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	if !strings.Contains(response.Body.String(), `go_order_metrics_collection_errors_total{collector="database"} 1`) {
		t.Fatalf("expected collector error metric, got:\n%s", response.Body.String())
	}
}
