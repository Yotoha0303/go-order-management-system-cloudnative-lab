package ordertimeout

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strconv"
	"strings"
	"time"

	"go-order-management-system-cloudnative-lab/internal/dao"
	"go-order-management-system-cloudnative-lab/internal/model"

	amqp "github.com/rabbitmq/amqp091-go"
	"gorm.io/gorm"
)

const (
	exchangeName     = "order.timeout"
	delayQueueName   = "order.timeout.delay"
	cancelQueueName  = "order.timeout.cancel"
	failedQueueName  = "order.timeout.failed"
	delayRoutingKey  = "delay"
	cancelRoutingKey = "cancel"
	failedRoutingKey = "failed"
)

type topology struct {
	exchange         string
	delayQueue       string
	cancelQueue      string
	failedQueue      string
	delayRoutingKey  string
	cancelRoutingKey string
	failedRoutingKey string
}

var productionTopology = topology{
	exchange:         exchangeName,
	delayQueue:       delayQueueName,
	cancelQueue:      cancelQueueName,
	failedQueue:      failedQueueName,
	delayRoutingKey:  delayRoutingKey,
	cancelRoutingKey: cancelRoutingKey,
	failedRoutingKey: failedRoutingKey,
}

type ExpiredOrderCanceller interface {
	CancelExpiredOrder(ctx context.Context, eventID string, orderID int64) error
}

type Config struct {
	URL                string
	ConnectTimeout     time.Duration
	ReconnectDelay     time.Duration
	OutboxPollInterval time.Duration
	OutboxRetryDelay   time.Duration
	PublishBatchSize   int
	ConsumerPrefetch   int
}

type Worker struct {
	config    Config
	db        *gorm.DB
	canceller ExpiredOrderCanceller
	logger    *slog.Logger
	now       func() time.Time
	topology  topology
}

func NewWorker(config Config, db *gorm.DB, canceller ExpiredOrderCanceller, logger *slog.Logger) (*Worker, error) {
	if strings.TrimSpace(config.URL) == "" {
		return nil, errors.New("create order timeout worker: RabbitMQ URL is required")
	}
	if config.ConnectTimeout <= 0 || config.ReconnectDelay <= 0 ||
		config.OutboxPollInterval <= 0 || config.OutboxRetryDelay <= 0 ||
		config.PublishBatchSize <= 0 || config.ConsumerPrefetch <= 0 {
		return nil, errors.New("create order timeout worker: durations and limits must be positive")
	}
	if db == nil {
		return nil, errors.New("create order timeout worker: database is required")
	}
	if isNilCanceller(canceller) {
		return nil, errors.New("create order timeout worker: order canceller is required")
	}
	if logger == nil {
		return nil, errors.New("create order timeout worker: logger is required")
	}
	return &Worker{
		config:    config,
		db:        db,
		canceller: canceller,
		logger:    logger,
		now:       time.Now,
		topology:  productionTopology,
	}, nil
}

func (w *Worker) Run(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		if err := w.runSession(ctx); err != nil && ctx.Err() == nil {
			w.logger.Error("order timeout RabbitMQ session stopped", "error", err)
		}
		if !waitFor(ctx, w.config.ReconnectDelay) {
			return nil
		}
	}
}

func (w *Worker) runSession(ctx context.Context) error {
	connection, err := amqp.DialConfig(w.config.URL, amqp.Config{
		Dial: amqp.DefaultDial(w.config.ConnectTimeout),
	})
	if err != nil {
		return fmt.Errorf("connect RabbitMQ: %w", err)
	}
	defer connection.Close()

	if err := declareTopology(connection, w.topology); err != nil {
		return err
	}

	publisher, err := connection.Channel()
	if err != nil {
		return fmt.Errorf("open order timeout publisher channel: %w", err)
	}
	defer publisher.Close()
	if err := publisher.Confirm(false); err != nil {
		return fmt.Errorf("enable order timeout publisher confirms: %w", err)
	}

	consumer, deliveries, err := openConsumer(connection, w.topology.cancelQueue, w.config.ConsumerPrefetch)
	if err != nil {
		return err
	}
	defer consumer.Close()

	sessionCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	errCh := make(chan error, 2)
	go func() { errCh <- w.publishLoop(sessionCtx, publisher) }()
	go func() { errCh <- w.consumeLoop(sessionCtx, deliveries) }()

	connectionClosed := connection.NotifyClose(make(chan *amqp.Error, 1))
	w.logger.Info("order timeout RabbitMQ worker connected")
	select {
	case <-ctx.Done():
		return nil
	case err := <-errCh:
		return err
	case err := <-connectionClosed:
		if err == nil {
			return errors.New("RabbitMQ connection closed")
		}
		return fmt.Errorf("RabbitMQ connection closed: %w", err)
	}
}

func declareTopology(connection *amqp.Connection, topology topology) error {
	channel, err := connection.Channel()
	if err != nil {
		return fmt.Errorf("open RabbitMQ topology channel: %w", err)
	}
	defer channel.Close()

	if err := channel.ExchangeDeclare(topology.exchange, "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare order timeout exchange: %w", err)
	}
	if _, err := channel.QueueDeclare(topology.delayQueue, true, false, false, false, amqp.Table{
		"x-dead-letter-exchange":    topology.exchange,
		"x-dead-letter-routing-key": topology.cancelRoutingKey,
	}); err != nil {
		return fmt.Errorf("declare order timeout delay queue: %w", err)
	}
	if err := channel.QueueBind(topology.delayQueue, topology.delayRoutingKey, topology.exchange, false, nil); err != nil {
		return fmt.Errorf("bind order timeout delay queue: %w", err)
	}
	if _, err := channel.QueueDeclare(topology.cancelQueue, true, false, false, false, amqp.Table{
		"x-dead-letter-exchange":    topology.exchange,
		"x-dead-letter-routing-key": topology.failedRoutingKey,
	}); err != nil {
		return fmt.Errorf("declare order timeout cancel queue: %w", err)
	}
	if err := channel.QueueBind(topology.cancelQueue, topology.cancelRoutingKey, topology.exchange, false, nil); err != nil {
		return fmt.Errorf("bind order timeout cancel queue: %w", err)
	}
	if _, err := channel.QueueDeclare(topology.failedQueue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare order timeout failed queue: %w", err)
	}
	if err := channel.QueueBind(topology.failedQueue, topology.failedRoutingKey, topology.exchange, false, nil); err != nil {
		return fmt.Errorf("bind order timeout failed queue: %w", err)
	}
	return nil
}

func openConsumer(connection *amqp.Connection, queue string, prefetch int) (*amqp.Channel, <-chan amqp.Delivery, error) {
	channel, err := connection.Channel()
	if err != nil {
		return nil, nil, fmt.Errorf("open order timeout consumer channel: %w", err)
	}
	if err := channel.Qos(prefetch, 0, false); err != nil {
		_ = channel.Close()
		return nil, nil, fmt.Errorf("configure order timeout consumer prefetch: %w", err)
	}
	deliveries, err := channel.Consume(queue, "", false, false, false, false, nil)
	if err != nil {
		_ = channel.Close()
		return nil, nil, fmt.Errorf("consume order timeout queue: %w", err)
	}
	return channel, deliveries, nil
}

func (w *Worker) publishLoop(ctx context.Context, channel *amqp.Channel) error {
	for {
		if err := w.publishBatch(ctx, channel); err != nil {
			return err
		}
		if !waitFor(ctx, w.config.OutboxPollInterval) {
			return nil
		}
	}
}

func (w *Worker) publishBatch(ctx context.Context, channel *amqp.Channel) error {
	events, err := dao.ListPendingOrderTimeoutOutbox(ctx, w.db, w.now(), w.config.PublishBatchSize)
	if err != nil {
		w.logger.Error("list order timeout outbox", "error", err)
		return nil
	}
	for _, outbox := range events {
		if err := w.publishEvent(ctx, channel, outbox); err != nil {
			_ = dao.MarkOrderTimeoutOutboxFailed(
				context.WithoutCancel(ctx),
				w.db,
				outbox.EventID,
				truncateError(err),
				w.now().Add(w.config.OutboxRetryDelay),
			)
			return err
		}
		if err := dao.MarkOrderTimeoutOutboxPublished(ctx, w.db, outbox.EventID, w.now()); err != nil {
			return fmt.Errorf("mark order timeout event published: %w", err)
		}
	}
	return nil
}

func (w *Worker) publishEvent(ctx context.Context, channel *amqp.Channel, outbox model.OrderTimeoutOutbox) error {
	event := Event{
		EventID:   outbox.EventID,
		OrderID:   outbox.OrderID,
		UserID:    outbox.UserID,
		TimeoutAt: outbox.TimeoutAt,
	}
	body, err := encodeEvent(event)
	if err != nil {
		return fmt.Errorf("encode order timeout event: %w", err)
	}
	confirmation, err := channel.PublishWithDeferredConfirmWithContext(
		ctx,
		w.topology.exchange,
		w.topology.delayRoutingKey,
		false,
		false,
		amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			ContentType:  "application/json",
			MessageId:    event.EventID,
			Timestamp:    w.now(),
			Expiration:   expirationMilliseconds(w.now(), event.TimeoutAt),
			Body:         body,
		},
	)
	if err != nil {
		return fmt.Errorf("publish order timeout event: %w", err)
	}
	if confirmation == nil {
		return errors.New("publish order timeout event: confirmation unavailable")
	}
	acknowledged, err := confirmation.WaitContext(ctx)
	if err != nil {
		return fmt.Errorf("wait for order timeout publish confirmation: %w", err)
	}
	if !acknowledged {
		return errors.New("RabbitMQ rejected order timeout event")
	}
	return nil
}

func (w *Worker) consumeLoop(ctx context.Context, deliveries <-chan amqp.Delivery) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case delivery, ok := <-deliveries:
			if !ok {
				return errors.New("order timeout delivery channel closed")
			}
			event, err := decodeEvent(delivery.Body)
			if err != nil {
				w.logger.Error("reject invalid order timeout event", "message_id", delivery.MessageId, "error", err)
				if nackErr := delivery.Nack(false, false); nackErr != nil {
					return fmt.Errorf("reject invalid order timeout event: %w", nackErr)
				}
				continue
			}
			if err := w.canceller.CancelExpiredOrder(ctx, event.EventID, event.OrderID); err != nil {
				w.logger.Error("cancel expired order", "event_id", event.EventID, "order_id", event.OrderID, "error", err)
				if !waitFor(ctx, w.config.OutboxRetryDelay) {
					return nil
				}
				if nackErr := delivery.Nack(false, true); nackErr != nil {
					return fmt.Errorf("requeue order timeout event: %w", nackErr)
				}
				continue
			}
			if err := delivery.Ack(false); err != nil {
				return fmt.Errorf("acknowledge order timeout event: %w", err)
			}
			w.logger.Info("processed order timeout event", "event_id", event.EventID, "order_id", event.OrderID)
		}
	}
}

func expirationMilliseconds(now, timeoutAt time.Time) string {
	remaining := timeoutAt.Sub(now).Milliseconds()
	if remaining < 1 {
		remaining = 1
	}
	return strconv.FormatInt(remaining, 10)
}

func waitFor(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func truncateError(err error) string {
	const maxLength = 255
	message := err.Error()
	if len(message) <= maxLength {
		return message
	}
	return message[:maxLength]
}

func isNilCanceller(canceller ExpiredOrderCanceller) bool {
	if canceller == nil {
		return true
	}
	value := reflect.ValueOf(canceller)
	return value.Kind() == reflect.Pointer && value.IsNil()
}
