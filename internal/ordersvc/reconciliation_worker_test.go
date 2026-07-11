package ordersvc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"gorm.io/gorm"
)

type stubReconciliationInventory struct {
	confirmErr   error
	releaseErr   error
	confirmCalls []string
	releaseCalls []string
}

func (stub *stubReconciliationInventory) Confirm(_ context.Context, reservationID string) (*InventoryReservation, error) {
	stub.confirmCalls = append(stub.confirmCalls, reservationID)
	if stub.confirmErr != nil {
		return nil, stub.confirmErr
	}
	return &InventoryReservation{ID: reservationID, Status: "confirmed"}, nil
}

func (stub *stubReconciliationInventory) Release(_ context.Context, reservationID string) (*InventoryReservation, error) {
	stub.releaseCalls = append(stub.releaseCalls, reservationID)
	if stub.releaseErr != nil {
		return nil, stub.releaseErr
	}
	return &InventoryReservation{ID: reservationID, Status: "released"}, nil
}

func TestReconciliationTriggerCreatesExplicitTasks(t *testing.T) {
	db := openReconciliationTestDB(t)
	cases := []struct {
		orderID int64
		from    string
		action  string
		status  string
	}{
		{1, OrderStatusReserving, ReconciliationActionReleaseInventoryAndFail, ReconciliationTaskPending},
		{2, OrderStatusCancelling, ReconciliationActionFinalizeCancel, ReconciliationTaskPending},
		{3, OrderStatusPaying, ReconciliationActionFinalizePayment, ReconciliationTaskPending},
		{4, OrderStatusPending, "unsupported_from_pending", ReconciliationTaskUnresolved},
	}
	for _, tc := range cases {
		insertReconciliationOrder(t, db, tc.orderID, tc.from, fmt.Sprintf("reservation-%d", tc.orderID))
		if err := db.Model(&Order{}).Where("id = ?", tc.orderID).Update("status", OrderStatusReconciliationRequired).Error; err != nil {
			t.Fatalf("transition order %d: %v", tc.orderID, err)
		}
	}

	var tasks []ReconciliationTask
	if err := db.Order("order_id ASC").Find(&tasks).Error; err != nil {
		t.Fatalf("load reconciliation tasks: %v", err)
	}
	if len(tasks) != len(cases) {
		t.Fatalf("expected %d tasks, got %d", len(cases), len(tasks))
	}
	for index, tc := range cases {
		if tasks[index].OrderID != tc.orderID || tasks[index].Action != tc.action || tasks[index].Status != tc.status {
			t.Fatalf("unexpected task %d: %+v", index, tasks[index])
		}
	}
}

func TestReconciliationTriggerRollsBackWithOrderTransition(t *testing.T) {
	db := openReconciliationTestDB(t)
	insertReconciliationOrder(t, db, 10, OrderStatusCancelling, "reservation-10")

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction: %v", tx.Error)
	}
	if err := tx.Model(&Order{}).Where("id = ?", 10).Update("status", OrderStatusReconciliationRequired).Error; err != nil {
		_ = tx.Rollback()
		t.Fatalf("update order in transaction: %v", err)
	}
	if err := tx.Rollback().Error; err != nil {
		t.Fatalf("rollback transaction: %v", err)
	}

	var order Order
	if err := db.First(&order, "id = ?", 10).Error; err != nil {
		t.Fatalf("load order after rollback: %v", err)
	}
	if order.Status != OrderStatusCancelling {
		t.Fatalf("order status escaped rollback: %s", order.Status)
	}
	var count int64
	if err := db.Model(&ReconciliationTask{}).Where("order_id = ?", 10).Count(&count).Error; err != nil {
		t.Fatalf("count tasks after rollback: %v", err)
	}
	if count != 0 {
		t.Fatalf("task escaped rollback: count=%d", count)
	}
}

func TestReconciliationWorkersUseExclusiveLeases(t *testing.T) {
	db := openReconciliationTestDB(t)
	now := time.Now().UTC()
	for orderID := int64(20); orderID < 23; orderID++ {
		insertReconciliationOrder(t, db, orderID, OrderStatusReconciliationRequired, fmt.Sprintf("reservation-%d", orderID))
		insertReconciliationTask(t, db, orderID, ReconciliationActionFinalizeCancel, ReconciliationTaskPending, now.Add(-time.Second))
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	inventory := &stubReconciliationInventory{}
	workerOne := mustReconciliationWorker(t, db, inventory, logger, "worker-one", 2)
	workerTwo := mustReconciliationWorker(t, db, inventory, logger, "worker-two", 2)

	first, err := workerOne.claim(context.Background())
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	second, err := workerTwo.claim(context.Background())
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if len(first) != 2 || len(second) != 1 {
		t.Fatalf("unexpected claim sizes: first=%d second=%d", len(first), len(second))
	}
	seen := make(map[uint64]struct{}, 3)
	for _, task := range append(first, second...) {
		if _, exists := seen[task.ID]; exists {
			t.Fatalf("task %d was claimed twice", task.ID)
		}
		seen[task.ID] = struct{}{}
	}

	if err := db.Model(&ReconciliationTask{}).
		Where("id > 0").
		Update("lease_until", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatalf("expire leases: %v", err)
	}
	workerThree := mustReconciliationWorker(t, db, inventory, logger, "worker-three", 3)
	reclaimed, err := workerThree.claim(context.Background())
	if err != nil {
		t.Fatalf("reclaim expired leases: %v", err)
	}
	if len(reclaimed) != 3 {
		t.Fatalf("expected 3 reclaimed tasks, got %d", len(reclaimed))
	}
	for _, task := range reclaimed {
		if task.Attempts != 2 || task.LeaseOwner != "worker-three" {
			t.Fatalf("unexpected reclaimed task: %+v", task)
		}
	}
}

func TestReconciliationWorkerRepairsSupportedActions(t *testing.T) {
	cases := []struct {
		name           string
		action         string
		targetStatus   string
		completeOutbox bool
		expectConfirm  bool
		expectRelease  bool
	}{
		{"release and fail", ReconciliationActionReleaseInventoryAndFail, OrderStatusFailed, false, false, true},
		{"finalize cancel", ReconciliationActionFinalizeCancel, OrderStatusCancelled, true, false, true},
		{"finalize payment", ReconciliationActionFinalizePayment, OrderStatusPaid, true, true, false},
	}
	for index, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := openReconciliationTestDB(t)
			orderID := int64(100 + index)
			reservationID := fmt.Sprintf("reservation-%d", orderID)
			insertReconciliationOrder(t, db, orderID, OrderStatusReconciliationRequired, reservationID)
			if tc.completeOutbox {
				insertReconciliationOutbox(t, db, orderID, OutboxPending)
			}
			insertReconciliationTask(t, db, orderID, tc.action, ReconciliationTaskPending, time.Now().Add(-time.Second))

			inventory := &stubReconciliationInventory{}
			worker := mustReconciliationWorker(t, db, inventory, slog.New(slog.NewTextHandler(io.Discard, nil)), "repair-worker", 1)
			tasks, err := worker.claim(context.Background())
			if err != nil || len(tasks) != 1 {
				t.Fatalf("claim repair task: tasks=%d err=%v", len(tasks), err)
			}
			if err := worker.processTask(context.Background(), &tasks[0]); err != nil {
				t.Fatalf("process repair task: %v", err)
			}

			var order Order
			if err := db.First(&order, "id = ?", orderID).Error; err != nil {
				t.Fatalf("load repaired order: %v", err)
			}
			if order.Status != tc.targetStatus {
				t.Fatalf("expected order status %s, got %s", tc.targetStatus, order.Status)
			}
			if order.FailureReason != "original failure" {
				t.Fatalf("failure history was erased: %q", order.FailureReason)
			}
			var task ReconciliationTask
			if err := db.First(&task, tasks[0].ID).Error; err != nil {
				t.Fatalf("load completed task: %v", err)
			}
			if task.Status != ReconciliationTaskCompleted || task.LeaseOwner != "" || task.LeaseUntil != nil {
				t.Fatalf("task not completed cleanly: %+v", task)
			}
			if tc.completeOutbox {
				var outbox TimeoutOutbox
				if err := db.First(&outbox, "order_id = ?", orderID).Error; err != nil {
					t.Fatalf("load completed outbox: %v", err)
				}
				if outbox.Status != OutboxCompleted {
					t.Fatalf("expected completed outbox, got %s", outbox.Status)
				}
			}
			if tc.expectConfirm != (len(inventory.confirmCalls) == 1) {
				t.Fatalf("unexpected confirm calls: %#v", inventory.confirmCalls)
			}
			if tc.expectRelease != (len(inventory.releaseCalls) == 1) {
				t.Fatalf("unexpected release calls: %#v", inventory.releaseCalls)
			}
		})
	}
}

func TestReconciliationWorkerKeepsFailedRepairRetryable(t *testing.T) {
	db := openReconciliationTestDB(t)
	insertReconciliationOrder(t, db, 200, OrderStatusReconciliationRequired, "reservation-200")
	insertReconciliationTask(t, db, 200, ReconciliationActionReleaseInventoryAndFail, ReconciliationTaskPending, time.Now().Add(-time.Second))
	inventoryErr := errors.New("inventory temporarily unavailable")
	inventory := &stubReconciliationInventory{releaseErr: inventoryErr}
	worker := mustReconciliationWorker(t, db, inventory, slog.New(slog.NewTextHandler(io.Discard, nil)), "retry-worker", 1)
	fixed := time.Date(2026, time.July, 11, 12, 0, 0, 0, time.UTC)
	worker.now = func() time.Time { return fixed }

	tasks, err := worker.claim(context.Background())
	if err != nil || len(tasks) != 1 {
		t.Fatalf("claim failed repair: tasks=%d err=%v", len(tasks), err)
	}
	if err := worker.processTask(context.Background(), &tasks[0]); !errors.Is(err, inventoryErr) {
		t.Fatalf("expected inventory failure, got %v", err)
	}

	var task ReconciliationTask
	if err := db.First(&task, tasks[0].ID).Error; err != nil {
		t.Fatalf("load failed task: %v", err)
	}
	if task.Status != ReconciliationTaskFailed || task.LeaseOwner != "" || task.LeaseUntil != nil {
		t.Fatalf("failed task is not retryable: %+v", task)
	}
	if !task.NextAttemptAt.Equal(fixed.Add(worker.cfg.RetryDelay)) {
		t.Fatalf("unexpected next attempt: %s", task.NextAttemptAt)
	}
	if task.LastError == "" {
		t.Fatal("failed task lost its error")
	}
	var order Order
	if err := db.First(&order, "id = ?", 200).Error; err != nil {
		t.Fatalf("load failed-repair order: %v", err)
	}
	if order.Status != OrderStatusReconciliationRequired {
		t.Fatalf("failed repair changed order status to %s", order.Status)
	}
}

func TestReconciliationWorkerKeepsUnsupportedActionVisible(t *testing.T) {
	db := openReconciliationTestDB(t)
	insertReconciliationOrder(t, db, 300, OrderStatusReconciliationRequired, "reservation-300")
	insertReconciliationTask(t, db, 300, "unknown_action", ReconciliationTaskPending, time.Now().Add(-time.Second))
	inventory := &stubReconciliationInventory{}
	worker := mustReconciliationWorker(t, db, inventory, slog.New(slog.NewTextHandler(io.Discard, nil)), "unresolved-worker", 1)

	tasks, err := worker.claim(context.Background())
	if err != nil || len(tasks) != 1 {
		t.Fatalf("claim unsupported task: tasks=%d err=%v", len(tasks), err)
	}
	if err := worker.processTask(context.Background(), &tasks[0]); err != nil {
		t.Fatalf("mark unsupported task unresolved: %v", err)
	}

	var task ReconciliationTask
	if err := db.First(&task, tasks[0].ID).Error; err != nil {
		t.Fatalf("load unresolved task: %v", err)
	}
	if task.Status != ReconciliationTaskUnresolved || task.LastError == "" || task.LeaseOwner != "" {
		t.Fatalf("unsupported task was hidden: %+v", task)
	}
	if len(inventory.confirmCalls) != 0 || len(inventory.releaseCalls) != 0 {
		t.Fatalf("unsupported action called inventory: confirm=%v release=%v", inventory.confirmCalls, inventory.releaseCalls)
	}
}

func mustReconciliationWorker(
	t *testing.T,
	db *gorm.DB,
	inventory reconciliationInventory,
	logger *slog.Logger,
	workerID string,
	batchSize int,
) *ReconciliationWorker {
	t.Helper()
	worker, err := NewReconciliationWorker(ReconciliationWorkerConfig{
		WorkerID:      workerID,
		BatchSize:     batchSize,
		LeaseDuration: time.Minute,
		RetryDelay:    time.Second,
		MaxRetryDelay: time.Minute,
		CallTimeout:   time.Second,
	}, db, inventory, logger)
	if err != nil {
		t.Fatalf("create reconciliation worker: %v", err)
	}
	return worker
}

func openReconciliationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := openOutboxLeaseTestDB(t)
	if err := db.Exec(`
		CREATE TABLE orders_v2 (
			id BIGINT NOT NULL,
			status VARCHAR(32) NOT NULL,
			reservation_id VARCHAR(36) NOT NULL DEFAULT '',
			failure_reason VARCHAR(500) NOT NULL DEFAULT '',
			created_at DATETIME(3) NOT NULL,
			updated_at DATETIME(3) NOT NULL,
			PRIMARY KEY (id),
			KEY idx_orders_status (status)
		) ENGINE=InnoDB
	`).Error; err != nil {
		t.Fatalf("create reconciliation orders table: %v", err)
	}
	if err := db.Exec(`
		CREATE TABLE order_reconciliation_tasks (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			order_id BIGINT NOT NULL,
			action VARCHAR(64) NOT NULL,
			status VARCHAR(20) NOT NULL,
			attempts INT NOT NULL DEFAULT 0,
			next_attempt_at DATETIME(3) NOT NULL,
			lease_owner VARCHAR(128) NOT NULL DEFAULT '',
			lease_until DATETIME(3) NULL,
			last_error VARCHAR(500) NOT NULL DEFAULT '',
			created_at DATETIME(3) NOT NULL,
			updated_at DATETIME(3) NOT NULL,
			PRIMARY KEY (id),
			UNIQUE KEY uk_order_reconciliation_order_action (order_id, action),
			KEY idx_order_reconciliation_claim (status, next_attempt_at, lease_until)
		) ENGINE=InnoDB
	`).Error; err != nil {
		t.Fatalf("create reconciliation tasks table: %v", err)
	}
	if err := db.Exec(`
		CREATE TRIGGER trg_test_orders_reconciliation
		AFTER UPDATE ON orders_v2
		FOR EACH ROW
		BEGIN
			IF NEW.status = 'reconciliation_required' AND OLD.status <> NEW.status THEN
				INSERT INTO order_reconciliation_tasks (
					order_id, action, status, attempts, next_attempt_at,
					lease_owner, lease_until, last_error, created_at, updated_at
				) VALUES (
					NEW.id,
					CASE OLD.status
						WHEN 'reserving' THEN 'release_inventory_and_fail'
						WHEN 'cancelling' THEN 'finalize_cancel'
						WHEN 'paying' THEN 'finalize_payment'
						ELSE CONCAT('unsupported_from_', OLD.status)
					END,
					CASE OLD.status
						WHEN 'reserving' THEN 'pending'
						WHEN 'cancelling' THEN 'pending'
						WHEN 'paying' THEN 'pending'
						ELSE 'unresolved'
					END,
					0, CURRENT_TIMESTAMP(3), '', NULL,
					CASE OLD.status
						WHEN 'reserving' THEN ''
						WHEN 'cancelling' THEN ''
						WHEN 'paying' THEN ''
						ELSE CONCAT('unsupported reconciliation transition from ', OLD.status)
					END,
					CURRENT_TIMESTAMP(3), CURRENT_TIMESTAMP(3)
				)
				ON DUPLICATE KEY UPDATE updated_at = CURRENT_TIMESTAMP(3);
			END IF;
		END
	`).Error; err != nil {
		t.Fatalf("create reconciliation trigger: %v", err)
	}
	return db
}

func insertReconciliationOrder(t *testing.T, db *gorm.DB, orderID int64, status string, reservationID string) {
	t.Helper()
	now := time.Now().UTC()
	if err := db.Exec(`
		INSERT INTO orders_v2 (id, status, reservation_id, failure_reason, created_at, updated_at)
		VALUES (?, ?, ?, 'original failure', ?, ?)
	`, orderID, status, reservationID, now, now).Error; err != nil {
		t.Fatalf("insert order %d: %v", orderID, err)
	}
}

func insertReconciliationTask(t *testing.T, db *gorm.DB, orderID int64, action string, status string, nextAttempt time.Time) {
	t.Helper()
	now := time.Now().UTC()
	if err := db.Exec(`
		INSERT INTO order_reconciliation_tasks
			(order_id, action, status, attempts, next_attempt_at, lease_owner, lease_until, last_error, created_at, updated_at)
		VALUES (?, ?, ?, 0, ?, '', NULL, '', ?, ?)
	`, orderID, action, status, nextAttempt, now, now).Error; err != nil {
		t.Fatalf("insert reconciliation task for order %d: %v", orderID, err)
	}
}

func insertReconciliationOutbox(t *testing.T, db *gorm.DB, orderID int64, status string) {
	t.Helper()
	now := time.Now().UTC()
	if err := db.Exec(`
		INSERT INTO order_timeout_outbox_v2
			(order_id, due_at, status, attempts, last_error, lease_owner, lease_until, next_attempt_at, created_at, updated_at)
		VALUES (?, ?, ?, 0, '', '', NULL, ?, ?, ?)
	`, orderID, now.Add(time.Minute), status, now, now, now).Error; err != nil {
		t.Fatalf("insert timeout outbox for order %d: %v", orderID, err)
	}
}
