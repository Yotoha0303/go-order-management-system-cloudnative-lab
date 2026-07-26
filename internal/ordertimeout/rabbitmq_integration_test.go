package ordertimeout

import (
	"context"
	"os"
	"testing"
	"time"

	"go-order-management-system-cloudnative-lab/internal/model"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	amqp "github.com/rabbitmq/amqp091-go"
)

func TestRabbitMQDelayAndDeadLetterTopology(t *testing.T) {
	if os.Getenv("RUN_RABBITMQ_TEST") != "1" {
		t.Skip("skip RabbitMQ integration test; set RUN_RABBITMQ_TEST=1 to run")
	}
	_ = godotenv.Load("../../.env")
	url := os.Getenv("RABBITMQ_URL")
	if url == "" {
		url = "amqp://order_app:order_dev_password@127.0.0.1:5672/"
	}

	connection, err := amqp.DialConfig(url, amqp.Config{Dial: amqp.DefaultDial(5 * time.Second)})
	if err != nil {
		t.Fatalf("connect RabbitMQ: %v", err)
	}
	t.Cleanup(func() { _ = connection.Close() })

	suffix := uuid.NewString()
	topology := topology{
		exchange:         "test.order.timeout." + suffix,
		delayQueue:       "test.order.timeout.delay." + suffix,
		cancelQueue:      "test.order.timeout.cancel." + suffix,
		failedQueue:      "test.order.timeout.failed." + suffix,
		delayRoutingKey:  "delay",
		cancelRoutingKey: "cancel",
		failedRoutingKey: "failed",
	}
	if err := declareTopology(connection, topology); err != nil {
		t.Fatalf("declareTopology: %v", err)
	}
	t.Cleanup(func() { deleteTestTopology(t, connection, topology) })

	consumer, deliveries, err := openConsumer(connection, topology.cancelQueue, 1)
	if err != nil {
		t.Fatalf("openConsumer: %v", err)
	}
	defer consumer.Close()
	publisher, err := connection.Channel()
	if err != nil {
		t.Fatalf("publisher channel: %v", err)
	}
	defer publisher.Close()
	if err := publisher.Confirm(false); err != nil {
		t.Fatalf("publisher confirms: %v", err)
	}

	now := time.Now()
	eventID := uuid.NewString()
	worker := &Worker{now: time.Now, topology: topology}
	if err := worker.publishEvent(context.Background(), publisher, model.OrderTimeoutOutbox{
		EventID:   eventID,
		OrderID:   101,
		UserID:    202,
		TimeoutAt: now.Add(200 * time.Millisecond),
	}); err != nil {
		t.Fatalf("publishEvent: %v", err)
	}

	select {
	case delivery := <-deliveries:
		event, err := decodeEvent(delivery.Body)
		if err != nil {
			t.Fatalf("decode delivered event: %v", err)
		}
		if event.EventID != eventID || event.OrderID != 101 {
			t.Fatalf("delivered event=%+v", event)
		}
		if time.Since(now) < 150*time.Millisecond {
			t.Fatalf("event delivered before TTL elapsed: %s", time.Since(now))
		}
		if err := delivery.Ack(false); err != nil {
			t.Fatalf("ack delivery: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for dead-lettered order timeout event")
	}
}

func deleteTestTopology(t *testing.T, connection *amqp.Connection, topology topology) {
	t.Helper()
	channel, err := connection.Channel()
	if err != nil {
		t.Logf("open cleanup channel: %v", err)
		return
	}
	defer channel.Close()
	for _, queue := range []string{topology.delayQueue, topology.cancelQueue, topology.failedQueue} {
		if _, err := channel.QueueDelete(queue, false, false, false); err != nil {
			t.Logf("delete queue %s: %v", queue, err)
		}
	}
	if err := channel.ExchangeDelete(topology.exchange, false, false); err != nil {
		t.Logf("delete exchange %s: %v", topology.exchange, err)
	}
}
