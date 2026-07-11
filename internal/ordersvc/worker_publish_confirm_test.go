package ordersvc

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
)

type stubTimeoutPublisher struct {
	err   error
	calls int
}

func (p *stubTimeoutPublisher) Publish(context.Context, TimeoutOutbox, []byte, time.Duration) error {
	p.calls++
	return p.err
}

func TestPublishPendingKeepsUnconfirmedEventRetryable(t *testing.T) {
	db := openOutboxLeaseTestDB(t)
	now := time.Now()
	if err := db.Exec(`
		INSERT INTO order_timeout_outbox_v2
			(order_id, due_at, status, attempts, last_error, lease_owner, lease_until, next_attempt_at, created_at, updated_at)
		VALUES (?, ?, ?, 0, '', '', NULL, ?, ?, ?)
	`, int64(101), now.Add(time.Minute), OutboxPending, now, now, now).Error; err != nil {
		t.Fatalf("insert outbox event: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	workerOne := &Worker{
		cfg: WorkerConfig{
			WorkerID:      "confirm-worker-one",
			BatchSize:     1,
			LeaseDuration: time.Minute,
			RetryDelay:    time.Millisecond,
		},
		db:     db,
		logger: logger,
	}
	failedPublisher := &stubTimeoutPublisher{err: errPublisherNacked}
	err := workerOne.publishPending(context.Background(), failedPublisher)
	if !errors.Is(err, errPublisherNacked) {
		t.Fatalf("expected publish failure, got %v", err)
	}
	if failedPublisher.calls != 1 {
		t.Fatalf("expected one publish call, got %d", failedPublisher.calls)
	}

	var failed TimeoutOutbox
	if err := db.Table(TimeoutOutbox{}.TableName()).First(&failed).Error; err != nil {
		t.Fatalf("load failed outbox event: %v", err)
	}
	if failed.Status != OutboxFailed {
		t.Fatalf("unconfirmed event must be failed, got %q", failed.Status)
	}
	if failed.Attempts != 1 {
		t.Fatalf("expected one failed attempt, got %d", failed.Attempts)
	}
	if failed.Status == OutboxPublished {
		t.Fatal("unconfirmed event was incorrectly marked published")
	}

	if err := db.Table(TimeoutOutbox{}.TableName()).
		Where("id = ?", failed.ID).
		Update("next_attempt_at", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatalf("make event retryable: %v", err)
	}

	workerTwo := &Worker{
		cfg: WorkerConfig{
			WorkerID:      "confirm-worker-two",
			BatchSize:     1,
			LeaseDuration: time.Minute,
			RetryDelay:    time.Millisecond,
		},
		db:     db,
		logger: logger,
	}
	confirmedPublisher := &stubTimeoutPublisher{}
	if err := workerTwo.publishPending(context.Background(), confirmedPublisher); err != nil {
		t.Fatalf("retry confirmed event: %v", err)
	}

	var published TimeoutOutbox
	if err := db.Table(TimeoutOutbox{}.TableName()).First(&published, failed.ID).Error; err != nil {
		t.Fatalf("load published outbox event: %v", err)
	}
	if published.Status != OutboxPublished {
		t.Fatalf("expected published status after confirmation, got %q", published.Status)
	}
	if published.Attempts != 2 {
		t.Fatalf("expected two total attempts, got %d", published.Attempts)
	}
}

func TestConfirmedAMQPPublisherReceivesBrokerAck(t *testing.T) {
	if os.Getenv("RUN_RABBITMQ_TEST") != "1" {
		t.Skip("set RUN_RABBITMQ_TEST=1 to run RabbitMQ integration tests")
	}
	url := os.Getenv("RABBITMQ_URL")
	if url == "" {
		url = "amqp://guest:guest@127.0.0.1:5672/"
	}

	conn, err := amqp.Dial(url)
	if err != nil {
		t.Fatalf("connect RabbitMQ: %v", err)
	}
	defer conn.Close()

	channel, err := conn.Channel()
	if err != nil {
		t.Fatalf("open RabbitMQ channel: %v", err)
	}
	defer channel.Close()

	suffix := uuid.NewString()
	exchange := "test.publisher.confirm." + suffix
	routingKey := "confirmed"
	if err := channel.ExchangeDeclare(exchange, "direct", false, true, false, false, nil); err != nil {
		t.Fatalf("declare test exchange: %v", err)
	}
	t.Cleanup(func() {
		_ = channel.ExchangeDelete(exchange, false, false)
	})

	queue, err := channel.QueueDeclare("", false, true, true, false, nil)
	if err != nil {
		t.Fatalf("declare test queue: %v", err)
	}
	if err := channel.QueueBind(queue.Name, routingKey, exchange, false, nil); err != nil {
		t.Fatalf("bind test queue: %v", err)
	}

	publisher, err := newConfirmedAMQPPublisher(channel, exchange, routingKey, 3*time.Second)
	if err != nil {
		t.Fatalf("create confirmed publisher: %v", err)
	}
	if err := publisher.Publish(
		context.Background(),
		TimeoutOutbox{ID: 1, OrderID: 1001},
		[]byte(`{"order_id":1001}`),
		time.Minute,
	); err != nil {
		t.Fatalf("publish with broker confirmation: %v", err)
	}

	delivery, ok, err := channel.Get(queue.Name, true)
	if err != nil {
		t.Fatalf("get confirmed message: %v", err)
	}
	if !ok {
		t.Fatal("expected one confirmed message in queue")
	}
	if delivery.MessageId != "1" {
		t.Fatalf("expected message id 1, got %q", delivery.MessageId)
	}
}
