package ordersvc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	platformtelemetry "go-order-management-system/internal/platform/telemetry"

	"go.opentelemetry.io/otel/attribute"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ReconciliationActionReleaseInventoryAndFail = "release_inventory_and_fail"
	ReconciliationActionFinalizeCancel          = "finalize_cancel"
	ReconciliationActionFinalizePayment         = "finalize_payment"

	ReconciliationTaskPending    = "pending"
	ReconciliationTaskFailed     = "failed"
	ReconciliationTaskCompleted  = "completed"
	ReconciliationTaskUnresolved = "unresolved"
)

var (
	ErrUnsupportedReconciliationAction = errors.New("unsupported reconciliation action")
	ErrReconciliationStateMismatch     = errors.New("order state does not match reconciliation action")
)

type ReconciliationTask struct {
	ID            uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	OrderID       int64      `gorm:"column:order_id;not null;index;uniqueIndex:uk_order_reconciliation_order_action,priority:1" json:"order_id"`
	Action        string     `gorm:"type:varchar(64);not null;uniqueIndex:uk_order_reconciliation_order_action,priority:2" json:"action"`
	Status        string     `gorm:"type:varchar(20);not null;index" json:"status"`
	Attempts      int        `gorm:"not null;default:0" json:"attempts"`
	NextAttemptAt time.Time  `gorm:"column:next_attempt_at;not null;index" json:"next_attempt_at"`
	LeaseOwner    string     `gorm:"column:lease_owner;type:varchar(128);not null;default:''" json:"lease_owner,omitempty"`
	LeaseUntil    *time.Time `gorm:"column:lease_until" json:"lease_until,omitempty"`
	LastError     string     `gorm:"column:last_error;type:varchar(500);not null;default:''" json:"last_error,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

func (ReconciliationTask) TableName() string { return "order_reconciliation_tasks" }

type reconciliationInventory interface {
	Confirm(context.Context, string) (*InventoryReservation, error)
	Release(context.Context, string) (*InventoryReservation, error)
}

type ReconciliationWorkerConfig struct {
	WorkerID      string
	PollInterval  time.Duration
	RetryDelay    time.Duration
	MaxRetryDelay time.Duration
	LeaseDuration time.Duration
	CallTimeout   time.Duration
	BatchSize     int
}

func (cfg ReconciliationWorkerConfig) normalized() ReconciliationWorkerConfig {
	cfg.WorkerID = strings.TrimSpace(cfg.WorkerID)
	if cfg.WorkerID == "" {
		cfg.WorkerID = fmt.Sprintf("reconciliation-worker-%d", time.Now().UnixNano())
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 2 * time.Second
	}
	if cfg.RetryDelay <= 0 {
		cfg.RetryDelay = 5 * time.Second
	}
	if cfg.MaxRetryDelay <= 0 {
		cfg.MaxRetryDelay = 5 * time.Minute
	}
	if cfg.MaxRetryDelay < cfg.RetryDelay {
		cfg.MaxRetryDelay = cfg.RetryDelay
	}
	if cfg.LeaseDuration <= 0 {
		cfg.LeaseDuration = 30 * time.Second
	}
	if cfg.CallTimeout <= 0 {
		cfg.CallTimeout = 10 * time.Second
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 10
	}
	if cfg.BatchSize > 100 {
		cfg.BatchSize = 100
	}
	return cfg
}

type ReconciliationWorker struct {
	cfg       ReconciliationWorkerConfig
	db        *gorm.DB
	inventory reconciliationInventory
	logger    *slog.Logger
	now       func() time.Time
}

func NewReconciliationWorker(
	cfg ReconciliationWorkerConfig,
	db *gorm.DB,
	inventory reconciliationInventory,
	logger *slog.Logger,
) (*ReconciliationWorker, error) {
	if db == nil {
		return nil, errors.New("reconciliation worker database is required")
	}
	if inventory == nil {
		return nil, errors.New("reconciliation worker inventory client is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &ReconciliationWorker{
		cfg:       cfg.normalized(),
		db:        db,
		inventory: inventory,
		logger:    logger,
		now:       time.Now,
	}, nil
}

func (worker *ReconciliationWorker) Run(ctx context.Context) error {
	if worker == nil {
		return errors.New("reconciliation worker is not configured")
	}
	worker.logger.Info(
		"order reconciliation worker running",
		"worker_id", worker.cfg.WorkerID,
		"poll_interval", worker.cfg.PollInterval,
		"lease_duration", worker.cfg.LeaseDuration,
		"batch_size", worker.cfg.BatchSize,
	)

	ticker := time.NewTicker(worker.cfg.PollInterval)
	defer ticker.Stop()
	for {
		if err := worker.processBatch(ctx); err != nil && !errors.Is(err, context.Canceled) {
			worker.logger.Warn("process reconciliation batch", "worker_id", worker.cfg.WorkerID, "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (worker *ReconciliationWorker) processBatch(ctx context.Context) error {
	ctx, span := platformtelemetry.Tracer().Start(ctx, "reconciliation.process_batch")
	defer span.End()
	tasks, err := worker.claim(ctx)
	span.SetAttributes(attribute.Int("go_order.batch_size", len(tasks)))
	if err != nil {
		return err
	}
	for index := range tasks {
		if err := worker.processTask(ctx, &tasks[index]); err != nil {
			worker.logger.Warn(
				"order reconciliation task failed",
				"worker_id", worker.cfg.WorkerID,
				"task_id", tasks[index].ID,
				"order_id", tasks[index].OrderID,
				"action", tasks[index].Action,
				"attempt", tasks[index].Attempts,
				"error", err,
			)
		}
	}
	return nil
}

func (worker *ReconciliationWorker) claim(ctx context.Context) ([]ReconciliationTask, error) {
	now := worker.now().UTC()
	leaseUntil := now.Add(worker.cfg.LeaseDuration)
	var tasks []ReconciliationTask
	err := worker.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status IN ?", []string{ReconciliationTaskPending, ReconciliationTaskFailed}).
			Where("next_attempt_at <= ?", now).
			Where("lease_until IS NULL OR lease_until < ?", now).
			Order("id ASC").
			Limit(worker.cfg.BatchSize).
			Find(&tasks).Error; err != nil {
			return err
		}
		for index := range tasks {
			result := tx.Model(&ReconciliationTask{}).
				Where("id = ?", tasks[index].ID).
				Updates(map[string]any{
					"attempts":    gorm.Expr("attempts + 1"),
					"lease_owner": worker.cfg.WorkerID,
					"lease_until": leaseUntil,
					"updated_at":  now,
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return fmt.Errorf("claim reconciliation task %d: no row updated", tasks[index].ID)
			}
			tasks[index].Attempts++
			tasks[index].LeaseOwner = worker.cfg.WorkerID
			tasks[index].LeaseUntil = &leaseUntil
		}
		return nil
	})
	return tasks, err
}

func (worker *ReconciliationWorker) processTask(parent context.Context, task *ReconciliationTask) error {
	parent, span := platformtelemetry.Tracer().Start(parent, "reconciliation.process_task")
	defer span.End()
	if task == nil {
		return errors.New("reconciliation task is required")
	}
	ctx, cancel := context.WithTimeout(parent, worker.cfg.CallTimeout)
	defer cancel()

	var order Order
	if err := worker.db.WithContext(ctx).First(&order, "id = ?", task.OrderID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return worker.markUnresolved(task, "order not found")
		}
		return worker.markFailed(task, err)
	}

	var err error
	switch task.Action {
	case ReconciliationActionReleaseInventoryAndFail:
		err = worker.releaseInventoryAndFail(ctx, task, &order)
	case ReconciliationActionFinalizeCancel:
		err = worker.finalizeCancel(ctx, task, &order)
	case ReconciliationActionFinalizePayment:
		err = worker.finalizePayment(ctx, task, &order)
	default:
		return worker.markUnresolved(task, fmt.Sprintf("%s: %s", ErrUnsupportedReconciliationAction, task.Action))
	}
	if err == nil {
		worker.logger.InfoContext(
			ctx,
			"order reconciliation task completed",
			"worker_id", worker.cfg.WorkerID,
			"task_id", task.ID,
			"order_id", task.OrderID,
			"action", task.Action,
			"attempt", task.Attempts,
		)
		return nil
	}
	if errors.Is(err, ErrReconciliationStateMismatch) || errors.Is(err, ErrUnsupportedReconciliationAction) {
		return worker.markUnresolved(task, err.Error())
	}
	return worker.markFailed(task, err)
}

func (worker *ReconciliationWorker) releaseInventoryAndFail(ctx context.Context, task *ReconciliationTask, order *Order) error {
	if order.Status == OrderStatusFailed {
		return worker.completeTask(task, OrderStatusFailed, false)
	}
	if order.Status != OrderStatusReconciliationRequired {
		return fmt.Errorf("%w: action=%s status=%s", ErrReconciliationStateMismatch, task.Action, order.Status)
	}
	if _, err := worker.inventory.Release(ctx, order.ReservationID); err != nil {
		return fmt.Errorf("release inventory reservation: %w", err)
	}
	return worker.completeTask(task, OrderStatusFailed, false)
}

func (worker *ReconciliationWorker) finalizeCancel(ctx context.Context, task *ReconciliationTask, order *Order) error {
	if order.Status == OrderStatusCancelled {
		return worker.completeTask(task, OrderStatusCancelled, true)
	}
	if order.Status != OrderStatusReconciliationRequired {
		return fmt.Errorf("%w: action=%s status=%s", ErrReconciliationStateMismatch, task.Action, order.Status)
	}
	if _, err := worker.inventory.Release(ctx, order.ReservationID); err != nil {
		return fmt.Errorf("confirm released inventory reservation: %w", err)
	}
	return worker.completeTask(task, OrderStatusCancelled, true)
}

func (worker *ReconciliationWorker) finalizePayment(ctx context.Context, task *ReconciliationTask, order *Order) error {
	if order.Status == OrderStatusPaid {
		return worker.completeTask(task, OrderStatusPaid, true)
	}
	if order.Status != OrderStatusReconciliationRequired {
		return fmt.Errorf("%w: action=%s status=%s", ErrReconciliationStateMismatch, task.Action, order.Status)
	}
	if _, err := worker.inventory.Confirm(ctx, order.ReservationID); err != nil {
		return fmt.Errorf("confirm inventory reservation: %w", err)
	}
	return worker.completeTask(task, OrderStatusPaid, true)
}

func (worker *ReconciliationWorker) completeTask(
	task *ReconciliationTask,
	targetStatus string,
	completeOutbox bool,
) error {
	ctx, cancel := worker.persistenceContext()
	defer cancel()
	now := worker.now().UTC()
	return worker.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Order{}).
			Where("id = ? AND status = ?", task.OrderID, OrderStatusReconciliationRequired).
			Update("status", targetStatus)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			var count int64
			if err := tx.Model(&Order{}).Where("id = ? AND status = ?", task.OrderID, targetStatus).Count(&count).Error; err != nil {
				return err
			}
			if count != 1 {
				return fmt.Errorf("%w: target=%s", ErrReconciliationStateMismatch, targetStatus)
			}
		}
		if completeOutbox {
			if err := tx.Model(&TimeoutOutbox{}).
				Where("order_id = ? AND status IN ?", task.OrderID, []string{OutboxPending, OutboxPublished, OutboxFailed}).
				Updates(map[string]any{"status": OutboxCompleted, "updated_at": now}).Error; err != nil {
				return err
			}
		}
		completed := tx.Model(&ReconciliationTask{}).
			Where("id = ? AND lease_owner = ?", task.ID, worker.cfg.WorkerID).
			Updates(map[string]any{
				"status":          ReconciliationTaskCompleted,
				"lease_owner":     "",
				"lease_until":     nil,
				"last_error":      "",
				"next_attempt_at": now,
				"updated_at":      now,
			})
		if completed.Error != nil {
			return completed.Error
		}
		if completed.RowsAffected != 1 {
			return fmt.Errorf("complete reconciliation task %d: lease ownership lost", task.ID)
		}
		return nil
	})
}

func (worker *ReconciliationWorker) markFailed(task *ReconciliationTask, cause error) error {
	ctx, cancel := worker.persistenceContext()
	defer cancel()
	now := worker.now().UTC()
	nextAttempt := now.Add(worker.retryDelay(task.Attempts))
	result := worker.db.WithContext(ctx).Model(&ReconciliationTask{}).
		Where("id = ? AND lease_owner = ?", task.ID, worker.cfg.WorkerID).
		Updates(map[string]any{
			"status":          ReconciliationTaskFailed,
			"lease_owner":     "",
			"lease_until":     nil,
			"last_error":      truncate(cause.Error(), 500),
			"next_attempt_at": nextAttempt,
			"updated_at":      now,
		})
	if result.Error != nil {
		return errors.Join(cause, result.Error)
	}
	if result.RowsAffected != 1 {
		return errors.Join(cause, fmt.Errorf("fail reconciliation task %d: lease ownership lost", task.ID))
	}
	return cause
}

func (worker *ReconciliationWorker) markUnresolved(task *ReconciliationTask, reason string) error {
	ctx, cancel := worker.persistenceContext()
	defer cancel()
	now := worker.now().UTC()
	result := worker.db.WithContext(ctx).Model(&ReconciliationTask{}).
		Where("id = ? AND lease_owner = ?", task.ID, worker.cfg.WorkerID).
		Updates(map[string]any{
			"status":          ReconciliationTaskUnresolved,
			"lease_owner":     "",
			"lease_until":     nil,
			"last_error":      truncate(reason, 500),
			"next_attempt_at": now,
			"updated_at":      now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("mark reconciliation task %d unresolved: lease ownership lost", task.ID)
	}
	worker.logger.Error(
		"order reconciliation task unresolved",
		"worker_id", worker.cfg.WorkerID,
		"task_id", task.ID,
		"order_id", task.OrderID,
		"action", task.Action,
		"attempt", task.Attempts,
		"reason", reason,
	)
	return nil
}

func (worker *ReconciliationWorker) persistenceContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

func (worker *ReconciliationWorker) retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := worker.cfg.RetryDelay
	for current := 1; current < attempt; current++ {
		if delay >= worker.cfg.MaxRetryDelay/2 {
			return worker.cfg.MaxRetryDelay
		}
		delay *= 2
	}
	if delay > worker.cfg.MaxRetryDelay {
		return worker.cfg.MaxRetryDelay
	}
	return delay
}
