package ordersvc

import (
	"context"
	"reflect"
	"testing"
	"time"

	"gorm.io/gorm"
)

type dryRunOrderState struct {
	ID            int64
	Status        string
	ReservationID string
	FailureReason string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type dryRunOutboxState struct {
	ID            uint64
	OrderID       int64
	DueAt         time.Time
	Status        string
	Attempts      int
	LastError     string
	LeaseOwner    string
	LeaseUntil    *time.Time
	NextAttemptAt time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type dryRunDatabaseState struct {
	Orders []dryRunOrderState
	Tasks  []ReconciliationTask
	Outbox []dryRunOutboxState
}

func TestReconciliationDryRunIsReadOnly(t *testing.T) {
	db := openReconciliationTestDB(t)
	fixed := time.Now().UTC().Truncate(time.Millisecond)

	insertReconciliationOrder(t, db, 1, OrderStatusReconciliationRequired, "reservation-1")
	insertReconciliationOrder(t, db, 2, OrderStatusReconciliationRequired, "reservation-2")
	insertReconciliationOrder(t, db, 3, OrderStatusPaid, "reservation-3")
	insertReconciliationOrder(t, db, 4, OrderStatusPending, "reservation-4")
	insertReconciliationOrder(t, db, 5, OrderStatusReconciliationRequired, "reservation-5")
	insertReconciliationOrder(t, db, 7, OrderStatusReconciliationRequired, "reservation-7")
	insertReconciliationOrder(t, db, 8, OrderStatusReconciliationRequired, "reservation-8")

	insertReconciliationTask(t, db, 1, ReconciliationActionReleaseInventoryAndFail, ReconciliationTaskPending, fixed.Add(-time.Second))
	insertReconciliationTask(t, db, 2, ReconciliationActionFinalizeCancel, ReconciliationTaskFailed, fixed.Add(-time.Second))
	insertReconciliationTask(t, db, 3, ReconciliationActionFinalizePayment, ReconciliationTaskPending, fixed.Add(-time.Second))
	insertReconciliationTask(t, db, 4, ReconciliationActionFinalizeCancel, ReconciliationTaskPending, fixed.Add(-time.Second))
	insertReconciliationTask(t, db, 5, "unknown_action", ReconciliationTaskPending, fixed.Add(-time.Second))
	insertReconciliationTask(t, db, 6, ReconciliationActionFinalizePayment, ReconciliationTaskPending, fixed.Add(-time.Second))
	insertReconciliationTask(t, db, 7, ReconciliationActionReleaseInventoryAndFail, ReconciliationTaskPending, fixed.Add(-time.Second))
	insertReconciliationTask(t, db, 8, ReconciliationActionReleaseInventoryAndFail, ReconciliationTaskPending, fixed.Add(time.Hour))
	insertReconciliationOutbox(t, db, 2, OutboxFailed)

	if err := db.Model(&ReconciliationTask{}).
		Where("order_id = ?", 7).
		Updates(map[string]any{"lease_owner": "active-worker", "lease_until": fixed.Add(time.Minute)}).Error; err != nil {
		t.Fatalf("activate task lease: %v", err)
	}

	before := loadDryRunDatabaseState(t, db)
	inventory := &stubReconciliationInventory{}
	worker := mustReconciliationWorker(t, db, inventory, nil, "dry-run-worker", 20)
	worker.now = func() time.Time { return fixed }

	report, err := worker.DryRun(context.Background())
	if err != nil {
		t.Fatalf("build dry-run report: %v", err)
	}
	if report.GeneratedAt != fixed {
		t.Fatalf("unexpected generated time: %s", report.GeneratedAt)
	}
	if report.EligibleCount != 6 || len(report.Items) != 6 {
		t.Fatalf("expected six eligible plans, got count=%d items=%d", report.EligibleCount, len(report.Items))
	}

	plans := make(map[int64]ReconciliationDryRunItem, len(report.Items))
	for _, item := range report.Items {
		plans[item.OrderID] = item
	}
	assertDryRunPlan(t, plans[1], ReconciliationPlanReady, OrderStatusFailed)
	assertDryRunPlan(t, plans[2], ReconciliationPlanReady, OrderStatusCancelled)
	assertDryRunPlan(t, plans[3], ReconciliationPlanAlreadyFinal, OrderStatusPaid)
	assertDryRunPlan(t, plans[4], ReconciliationPlanStateMismatch, OrderStatusCancelled)
	assertDryRunPlan(t, plans[5], ReconciliationPlanUnsupported, "")
	assertDryRunPlan(t, plans[6], ReconciliationPlanOrderMissing, OrderStatusPaid)
	if _, exists := plans[7]; exists {
		t.Fatal("actively leased task appeared in dry-run plan")
	}
	if _, exists := plans[8]; exists {
		t.Fatal("future task appeared in dry-run plan")
	}

	if len(inventory.confirmCalls) != 0 || len(inventory.releaseCalls) != 0 {
		t.Fatalf("dry-run called Inventory: confirm=%v release=%v", inventory.confirmCalls, inventory.releaseCalls)
	}
	after := loadDryRunDatabaseState(t, db)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("dry-run changed database state:\nbefore=%#v\nafter=%#v", before, after)
	}
}

func TestReconciliationDryRunEmptyEligibleSet(t *testing.T) {
	db := openReconciliationTestDB(t)
	inventory := &stubReconciliationInventory{}
	worker := mustReconciliationWorker(t, db, inventory, nil, "empty-dry-run", 10)

	report, err := worker.DryRun(context.Background())
	if err != nil {
		t.Fatalf("empty dry-run: %v", err)
	}
	if report.EligibleCount != 0 || len(report.Items) != 0 {
		t.Fatalf("expected empty report, got %+v", report)
	}
	if len(inventory.confirmCalls) != 0 || len(inventory.releaseCalls) != 0 {
		t.Fatal("empty dry-run called Inventory")
	}
}

func assertDryRunPlan(t *testing.T, item ReconciliationDryRunItem, classification string, target string) {
	t.Helper()
	if item.Classification != classification {
		t.Fatalf("order %d: expected classification %s, got %+v", item.OrderID, classification, item)
	}
	if item.IntendedTargetState != target {
		t.Fatalf("order %d: expected target %s, got %+v", item.OrderID, target, item)
	}
}

func loadDryRunDatabaseState(t *testing.T, db *gorm.DB) dryRunDatabaseState {
	t.Helper()
	var state dryRunDatabaseState
	if err := db.Table("orders_v2").Order("id ASC").Scan(&state.Orders).Error; err != nil {
		t.Fatalf("snapshot orders: %v", err)
	}
	if err := db.Order("id ASC").Find(&state.Tasks).Error; err != nil {
		t.Fatalf("snapshot tasks: %v", err)
	}
	if err := db.Table(TimeoutOutbox{}.TableName()).Order("id ASC").Scan(&state.Outbox).Error; err != nil {
		t.Fatalf("snapshot outbox: %v", err)
	}
	return state
}
