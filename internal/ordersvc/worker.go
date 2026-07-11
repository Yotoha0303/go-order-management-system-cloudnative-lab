package ordersvc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	timeoutExchange    = "order.timeout.v2"
	timeoutDelayQueue  = "order.timeout.delay.v2"
	timeoutCancelQueue = "order.timeout.cancel.v2"
)

type WorkerConfig struct {
	URL             string
	ReconnectDelay  time.Duration
	PollInterval    time.Duration
	RetryDelay      time.Duration
	BatchSize       int
	Prefetch        int
	OrderServiceURL string
	InternalToken   string
	CallTimeout     time.Duration
	WorkerID        string
	LeaseDuration   time.Duration
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
	if logger == nil {
		logger = slog.Default()
	}
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

	channel, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("open rabbitmq channel: %w", err)
	}
	defer channel.Close()

	if err := declareTimeoutTopology(channel); err != nil {
		return err
	}
	if err := channel.Qos(w.cfg.Prefetch, 0, false); err != nil {
		return fmt.Errorf("set consumer qos: %w", err)
	}

	deliveries, err := channel.Consume(timeoutCancelQueue, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume timeout queue: %w", err)
	}

	closed := make(chan *amqp.Error, 1)
	channel.NotifyClose(closed)
	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()

	w.logger.Info("timeout worker connected", "worker_id", w.cfg.WorkerID)
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-closed:
			if err == nil {
				return errors.New("rabbitmq channel closed")
			}
			return err
		case <-ticker.C:
			if err := w.publishPending(ctx, channel); err != nil {
				w.logger.Error("publish timeout outbox batch", "worker_id", w.cfg.WorkerID, "error", err)
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

func (w *Worker) publishPending(ctx context.Context, channel *amqp.Channel) error {
	events, err := w.claimPending(ctx)
	if err != nil {
		return err
	}

	for _, event := range events {
		payload, marshalErr := json.Marshal(struct {
			OrderID int64 `json:"order_id"`
		}{OrderID: event.OrderID})
		if marshalErr != nil {
			_ = w.markOutboxFailure(event.ID, marshalErr)
			continue
		}

		delay := time.Until(event.DueAt)
		if delay < time.Second {
			delay = time.Second
		}
		publishCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		publishErr := channel.PublishWithContext(publishCtx, timeoutExchange, "delay", false, false, amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			ContentType:  "application/json",
			MessageId:    strconv.FormatUint(event.ID, 10),
			Expiration:   strconv.FormatInt(delay.Milliseconds(), 10),
			Timestamp:    time.Now(),
			Body:         payload,
		})
		cancel()
		if publishErr != nil {
			_ = w.markOutboxFailure(event.ID, publishErr)
			continue
		}
		if err := w.markOutboxPublished(ctx, event.ID); err != nil {
			return err
		}
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
	var payload struct {
		OrderID int64 `json:"order_id"`
	}
	if err := json.Unmarshal(delivery.Body, &payload); err != nil || payload.OrderID <= 0 {
		w.logger.Error("invalid timeout message", "error", err)
		_ = delivery.Reject(false)
		return
	}

	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	err := w.orderClient.TimeoutCancel(callCtx, payload.OrderID)
	cancel()
	if err != nil {
		w.logger.Error("cancel timed out order", "order_id", payload.OrderID, "error", err)
		_ = w.db.WithContext(context.Background()).Model(&TimeoutOutbox{}).
			Where("order_id = ?", payload.OrderID).
			Updates(map[string]any{
				"attempts":   gorm.Expr("attempts + 1"),
				"last_error": truncate(err.Error(), 500),
			}).Error
		time.Sleep(w.cfg.RetryDelay)
		_ = delivery.Nack(false, true)
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
		w.logger.Error("mark timeout event completed", "order_id", payload.OrderID, "error", err)
		_ = delivery.Nack(false, true)
		return
	}
	_ = delivery.Ack(false)
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
