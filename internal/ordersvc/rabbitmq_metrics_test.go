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
		switch request.URL.EscapedPath() {
		case "/api/queues/team%2Fblue/" + timeoutDelayQueue:
			_, _ = writer.Write([]byte(`{"messages":3,"messages_ready":2,"messages_unacknowledged":1,"consumers":0}`))
		case "/api/queues/team%2Fblue/" + timeoutCancelQueue:
			_, _ = writer.Write([]byte(`{"messages":1,"messages_ready":0,"messages_unacknowledged":1,"consumers":2}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	collector, err := RabbitMQManagementPrometheusCollector(
		server.URL,
		"amqp://metrics-user:metrics-password@rabbitmq:5672/team%2Fblue",
		time.Second,
	)
	if err != nil {
		t.Fatalf("create collector: %v", err)
	}
	registry := platformmetrics.NewRegistry()
	if err := collector.Collect(context.Background(), registry); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}
	output := string(registry.Gather())
	for _, expected := range []string{
		"go_order_rabbitmq_management_up 1",
		"go_order_rabbitmq_queue_messages{queue_role=\"delay\",state=\"total\"} 3",
		"go_order_rabbitmq_queue_messages{queue_role=\"cancel\",state=\"unacknowledged\"} 1",
		"go_order_rabbitmq_queue_consumers{queue_role=\"cancel\"} 2",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("missing metric %q in:\n%s", expected, output)
		}
	}
	if strings.Contains(output, timeoutDelayQueue) || strings.Contains(output, timeoutCancelQueue) {
		t.Fatalf("raw queue name leaked into metric labels: %s", output)
	}
}

func TestRabbitMQManagementTargetUsesDefaultVhostAndDecodedCredentials(t *testing.T) {
	username, password, vhost, err := rabbitMQManagementTarget("amqps://metrics%2Duser:p%40ssword@rabbitmq:5671/")
	if err != nil {
		t.Fatalf("parse management target: %v", err)
	}
	if username != "metrics-user" || password != "p@ssword" || vhost != "/" {
		t.Fatalf("unexpected target: username=%q password=%q vhost=%q", username, password, vhost)
	}
}

func TestRabbitMQManagementCollectorRejectsAMQPURLWithoutCredentials(t *testing.T) {
	if _, err := RabbitMQManagementPrometheusCollector(
		"http://rabbitmq:15672",
		"amqp://rabbitmq:5672/",
		time.Second,
	); err == nil {
		t.Fatal("expected missing AMQP credentials to be rejected")
	}
}

func TestRabbitMQManagementCollectorMarksCollectionDown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	collector, err := RabbitMQManagementPrometheusCollector(
		server.URL,
		"amqp://metrics-user:metrics-password@rabbitmq:5672/",
		time.Second,
	)
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
