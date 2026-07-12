package ordersvc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	platformmetrics "go-order-management-system/internal/platform/metrics"
)

func TestRabbitMQManagementCollectorExportsBoundedQueueRoles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		username, password, ok := request.BasicAuth()
		if !ok || username != "metrics-user" || password != "metrics-password" {
			http.Error(writer, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch request.URL.Path {
		case "/api/queues///" + timeoutDelayQueue:
			_, _ = writer.Write([]byte(`{"messages":3,"messages_ready":2,"messages_unacknowledged":1,"consumers":0}`))
		case "/api/queues///" + timeoutCancelQueue:
			_, _ = writer.Write([]byte(`{"messages":1,"messages_ready":0,"messages_unacknowledged":1,"consumers":2}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	collector, err := RabbitMQManagementPrometheusCollector(server.URL, "metrics-user", "metrics-password", time.Second)
	if err != nil {
		t.Fatalf("create collector: %v", err)
	}
	registry := platformmetrics.NewRegistry()
	if err := collector.Collect(context.Background(), registry); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}
	output := string(registry.Gather())
	for _, expected := range []string{
		`go_order_rabbitmq_management_up 1`,
		`go_order_rabbitmq_queue_messages{queue_role="delay",state="total"} 3`,
		`go_order_rabbitmq_queue_messages{queue_role="cancel",state="unacknowledged"} 1`,
		`go_order_rabbitmq_queue_consumers{queue_role="cancel"} 2`,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("missing metric %q in:\n%s", expected, output)
		}
	}
	if strings.Contains(output, timeoutDelayQueue) || strings.Contains(output, timeoutCancelQueue) {
		t.Fatalf("raw queue name leaked into metric labels: %s", output)
	}
}

func TestRabbitMQManagementCollectorMarksCollectionDown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	collector, err := RabbitMQManagementPrometheusCollector(server.URL, "metrics-user", "metrics-password", time.Second)
	if err != nil {
		t.Fatalf("create collector: %v", err)
	}
	registry := platformmetrics.NewRegistry()
	if err := collector.Collect(context.Background(), registry); err == nil {
		t.Fatal("expected collection failure")
	}
	if output := string(registry.Gather()); !strings.Contains(output, "go_order_rabbitmq_management_up 0") {
		t.Fatalf("management-down gauge missing: %s", output)
	}
}
