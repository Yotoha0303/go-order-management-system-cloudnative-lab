package ordersvc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	platformtelemetry "go-order-management-system/internal/platform/telemetry"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel/attribute"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	timeoutExchange    = "order.timeout.v2"
	timeoutDelayQueue  = "order.timeout.delay.v2"
	timeoutCancelQueue = "order.timeout.cancel.v2"
)

type WorkerConfig struct {
	URL                   string
	ReconnectDelay        time.Duration
	PollInterval          time.Duration
	RetryDelay            time.Duration
	BatchSize             int
	Prefetch              int
	OrderServiceURL       string
	InternalToken         string
	CallTimeout           time.Duration
	WorkerID              string
	LeaseDuration         time.Duration
	PublishConfirmTimeout time.Duration
}

type Worker struct {
	cfg         WorkerConfig
	db          *gorm.DB
	orderClient *OrderServiceClient
	logger      *slog.Logger
}

func NewWorker(cfg WorkerConfig, db *gorm.DB, logger *slog.Logger) (*Worker, error) {
	cfg.URL = strings.TrimSpace(cfg.URL)
	cfg.OrderServiceURL = strings.TrimRight(strings.TrimSpace(cfg.OrderServiceURL), "/")
	cfg.InternalToken = strings.TrimSpace(cfg.InternalToken)
	cfg.WorkerID = strings.TrimSpace(cfg.WorkerID)
	if cfg.URL == "" || cfg.OrderServiceURL == "" || cfg.InternalToken == "" || db == nil {
		return nil, errors.New("order timeout worker configuration is incomplete")
	}
	if cfg.ReconnectDelay <= 0 {
		cfg.ReconnectDelay = 5 * time.Second
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = time.Second
	}
	if cfg.RetryDelay <= 0 {
		cfg.RetryDelay = 5 * time.Second
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 100
	}
	if cfg.Prefetch <= 0 {
		cfg.Prefetch = 10
	}
	if cfg.CallTimeout <= 0 {
		cfg.CallTimeout = 10 * time.Second
	}
	if cfg.WorkerID == "" {
		cfg.WorkerID = uuid.NewString()
	}
	if cfg.LeaseDuration <= 0 {
		cfg.LeaseDuration = 30 * time.Second
	}
	if cfg.PublishConfirmTimeout <= 0 {
		cfg.PublishConfirmTimeout = 5 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	setRabbitMQSessionUp(false)
	return &Worker{
		cfg:         cfg,
		db:          db,
		orderClient: NewOrderServiceClient(cfg.OrderServiceURL, cfg.InternalToken, cfg.CallTimeout),
		logger:      logger,
	}, nil
}

func (w *Worker) Run(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		if err := w.runSession(ctx); err != nil {
			w.logger.Error("timeout worker session failed", "worker_id", w.cfg.WorkerID, "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(w.cfg.ReconnectDelay):
		}
	}
}

func (w *Worker) runSession(ctx context.Context) error {
	conn, err := amqp.Dial(w.cfg.URL)
	if err != nil {
		return fmt.Errorf("connect rabbitmq: %w", err)
	}
	defer conn.Close()

	consumerChannel, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("open rabbitmq consumer channel: %w", err)
	}
	defer consumerChannel.Close()

	publisherChannel, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("open rabbitmq publisher channel: %w", err)
	}
	defer publisherChannel.Close()

	if err := declareTimeoutTopology(consumerChannel); err != nil {
		return err
	}
	publisher, err := newConfirmedAMQPPublisher(
		publisherChannel,
		timeoutExchange,
		"delay",
		w.cfg.PublishConfirmTimeout,
	)
	if err != nil {
		return err
	}
	if err := consumerChannel.Qos(w.cfg.Prefetch, 0, false); err != nil {
		return fmt.Errorf("set consumer qos: %w", err)
	}

	deliveries, err := consumerChannel.Consume(timeoutCancelQueue, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume timeout queue: %w", err)
	}
	setRabbitMQSessionUp(true)
	defer setRabbitMQSessionUp(false)

	consumerClosed := consumerChannel.NotifyClose(make(chan *amqp.Error, 1))
	publisherClosed := publisherChannel.NotifyClose(make(chan *amqp.Error, 1))
	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()

	w.logger.Info(
		"timeout worker connected",
		"worker_id", w.cfg.WorkerID,
		"publisher_confirms", true,
		"publisher_confirm_timeout", w.cfg.PublishConfirmTimeout,
	)
	for {
		select {
		case <-ctx.Done():
			return nil
		case closeErr, ok := <-consumerClosed:
			if !ok || closeErr == nil {
				return errors.New("rabbitmq consumer channel closed")
			}
			return fmt.Errorf("rabbitmq consumer channel closed: %w", closeErr)
		case closeErr, ok := <-publisherClosed:
			if !ok || closeErr == nil {
				return errors.New("rabbitmq publisher channel closed")
			}
			return fmt.Errorf("rabbitmq publisher channel closed: %w", closeErr)
		case <-ticker.C:
			if err := w.publishPending(ctx, publisher); err != nil {
				return err
			}
		case delivery, ok := <-deliveries:
			if !ok {
				return errors.New("timeout delivery channel closed")
			}
			w.handleDelivery(ctx, delivery)
		}
	}
}

func declareTimeoutTopology(channel *amqp.Channel) error {
	if err := channel.ExchangeDeclare(timeoutExchange, "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare timeout exchange: %w", err)
	}
	delayArgs := amqp.Table{
		"x-dead-letter-exchange":    timeoutExchange,
		"x-dead-letter-routing-key": "cancel",
	}
	if _, err := channel.QueueDeclare(timeoutDelayQueue, true, false, false, false, delayArgs); err != nil {
		return fmt.Errorf("declare timeout delay queue: %w", err)
	}
	if err := channel.QueueBind(timeoutDelayQueue, "delay", timeoutExchange, false, nil); err != nil {
		return fmt.Errorf("bind timeout delay queue: %w", err)
	}
	if _, err := channel.QueueDeclare(timeoutCancelQueue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare timeout cancel queue: %w", err)
	}
	if err := channel.QueueBind(timeoutCancelQueue, "cancel", timeoutExchange, false, nil); err != nil {
		return fmt.Errorf("bind timeout cancel queue: %w", err)
	}
	return nil
}

func (w *Worker) publishPending(ctx context.Context, publisher timeoutEventPublisher) error {
	ctx, span := platformtelemetry.Tracer().Start(ctx, "timeout_worker.publish_batch")
	defer span.End()
	events, err := w.claimPending(ctx)
	span.SetAttributes(attribute.Int("go_order.batch_size", len(events)))
	if err != nil {
		return err
	}

	for index, event := range events {
		payload, marshalErr := json.Marshal(struct {
			OrderID int64 `json:"order_id"`
		}{OrderID: event.OrderID})
		if marshalErr != nil {
			if err := w.markOutboxFailure(event.ID, marshalErr); err != nil {
				return err
			}
			continue
		}

		delay := time.Until(event.DueAt)
		publishErr := publisher.Publish(ctx, event, payload, delay)
		if publishErr != nil {
			if err := w.markOutboxFailure(event.ID, publishErr); err != nil {
				return fmt.Errorf("record outbox publish failure: %w", err)
			}
			if err := w.releaseClaimedLeases(ctx, events[index+1:]); err != nil {
				return fmt.Errorf("release unprocessed outbox leases: %w", err)
			}
			w.logger.ErrorContext(
				ctx,
				"timeout outbox publish not confirmed",
				"event_id", event.ID,
				"order_id", event.OrderID,
				"worker_id", w.cfg.WorkerID,
				"attempt", event.Attempts+1,
				"confirmation_outcome", "failed",
				"error", publishErr,
			)
			return fmt.Errorf("publish timeout outbox event %d: %w", event.ID, publishErr)
		}
		if err := w.markOutboxPublished(ctx, event.ID); err != nil {
			return err
		}
		w.logger.InfoContext(
			ctx,
			"timeout outbox publish confirmed",
			"event_id", event.ID,
			"order_id", event.OrderID,
			"worker_id", w.cfg.WorkerID,
			"attempt", event.Attempts+1,
			"confirmation_outcome", "ack",
		)
	}
	return nil
}

func (w *Worker) claimPending(ctx context.Context) ([]TimeoutOutbox, error) {
	now := time.Now()
	leaseUntil := now.Add(w.cfg.LeaseDuration)
	var events []TimeoutOutbox

	err := w.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Table(TimeoutOutbox{}.TableName()).
			Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status IN ?", []string{OutboxPending, OutboxFailed}).
			Where("next_attempt_at <= ?", now).
			Where("lease_until IS NULL OR lease_until < ?", now).
			Order("id ASC").
			Limit(w.cfg.BatchSize).
			Find(&events).Error; err != nil {
			return err
		}
		if len(events) == 0 {
			return nil
		}

		ids := make([]uint64, 0, len(events))
		for _, event := range events {
			ids = append(ids, event.ID)
		}
		result := tx.Table(TimeoutOutbox{}.TableName()).
			Where("id IN ?", ids).
			Where("status IN ?", []string{OutboxPending, OutboxFailed}).
			Where("lease_until IS NULL OR lease_until < ?", now).
			Updates(map[string]any{
				"lease_owner": w.cfg.WorkerID,
				"lease_until": leaseUntil,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != int64(len(events)) {
			return fmt.Errorf("claim timeout outbox batch: expected %d rows, updated %d", len(events), result.RowsAffected)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return events, nil
}

func (w *Worker) releaseClaimedLeases(ctx context.Context, events []TimeoutOutbox) error {
	if len(events) == 0 {
		return nil
	}
	ids := make([]uint64, 0, len(events))
	for _, event := range events {
		ids = append(ids, event.ID)
	}
	return w.db.WithContext(ctx).Table(TimeoutOutbox{}.TableName()).
		Where("id IN ? AND lease_owner = ?", ids, w.cfg.WorkerID).
		Updates(map[string]any{
			"lease_owner": "",
			"lease_until": nil,
		}).Error
}

func (w *Worker) markOutboxPublished(ctx context.Context, eventID uint64) error {
	result := w.db.WithContext(ctx).Table(TimeoutOutbox{}.TableName()).
		Where("id = ? AND lease_owner = ?", eventID, w.cfg.WorkerID).
		Updates(map[string]any{
			"status":          OutboxPublished,
			"attempts":        gorm.Expr("attempts + 1"),
			"last_error":      "",
			"lease_owner":     "",
			"lease_until":     nil,
			"next_attempt_at": time.Now(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("timeout outbox lease was lost before publish completion")
	}
	return nil
}

func (w *Worker) handleDelivery(ctx context.Context, delivery amqp.Delivery) {
	ctx, span := platformtelemetry.Tracer().Start(ctx, "timeout_worker.consume")
	defer span.End()
	recordRabbitMQDelivery("received")

	var payload struct {
		OrderID int64 `json:"order_id"`
	}
	if err := json.Unmarshal(delivery.Body, &payload); err != nil || payload.OrderID <= 0 {
		recordRabbitMQDelivery("processing_failure")
		w.logger.ErrorContext(ctx, "invalid timeout message", "error", err)
		if rejectErr := delivery.Reject(false); rejectErr != nil {
			recordRabbitMQDelivery("settlement_error")
			w.logger.ErrorContext(ctx, "reject invalid timeout message", "error", rejectErr)
		} else {
			recordRabbitMQDelivery("rejected")
		}
		return
	}

	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	err := w.orderClient.TimeoutCancel(callCtx, payload.OrderID)
	cancel()
	if err != nil {
		recordRabbitMQDelivery("processing_failure")
		w.logger.ErrorContext(ctx, "cancel timed out order", "order_id", payload.OrderID, "error", err)
		_ = w.db.WithContext(context.Background()).Model(&TimeoutOutbox{}).
			Where("order_id = ?", payload.OrderID).
			Updates(map[string]any{
				"attempts":   gorm.Expr("attempts + 1"),
				"last_error": truncate(err.Error(), 500),
			}).Error
		time.Sleep(w.cfg.RetryDelay)
		if nackErr := delivery.Nack(false, true); nackErr != nil {
			recordRabbitMQDelivery("settlement_error")
			w.logger.ErrorContext(ctx, "requeue failed timeout message", "error", nackErr)
		} else {
			recordRabbitMQDelivery("requeued")
		}
		return
	}

	if err := w.db.WithContext(context.Background()).Model(&TimeoutOutbox{}).
		Where("order_id = ?", payload.OrderID).
		Updates(map[string]any{
			"status":      OutboxCompleted,
			"last_error":  "",
			"lease_owner": "",
			"lease_until": nil,
		}).Error; err != nil {
		recordRabbitMQDelivery("processing_failure")
		w.logger.ErrorContext(ctx, "mark timeout event completed", "order_id", payload.OrderID, "error", err)
		if nackErr := delivery.Nack(false, true); nackErr != nil {
			recordRabbitMQDelivery("settlement_error")
			w.logger.ErrorContext(ctx, "requeue timeout message after persistence failure", "error", nackErr)
		} else {
			recordRabbitMQDelivery("requeued")
		}
		return
	}
	if ackErr := delivery.Ack(false); ackErr != nil {
		recordRabbitMQDelivery("settlement_error")
		w.logger.ErrorContext(ctx, "acknowledge timeout message", "error", ackErr)
		return
	}
	recordRabbitMQDelivery("acknowledged")
}

func (w *Worker) markOutboxFailure(eventID uint64, cause error) error {
	result := w.db.WithContext(context.Background()).Table(TimeoutOutbox{}.TableName()).
		Where("id = ? AND lease_owner = ?", eventID, w.cfg.WorkerID).
		Updates(map[string]any{
			"status":          OutboxFailed,
			"attempts":        gorm.Expr("attempts + 1"),
			"last_error":      truncate(cause.Error(), 500),
			"lease_owner":     "",
			"lease_until":     nil,
			"next_attempt_at": time.Now().Add(w.cfg.RetryDelay),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("timeout outbox lease was lost before failure update")
	}
	return nil
}
