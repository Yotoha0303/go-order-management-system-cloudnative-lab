package ordersvc

import (
	"context"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func TestReliabilityReporterSnapshot(t *testing.T) {
	db := openOutboxLeaseTestDB(t)
	createReliabilityOrdersTable(t, db)
	fixed := time.Date(2026, time.July, 11, 12, 0, 0, 0, time.UTC)

	outboxRows := []struct {
		orderID     int64
		dueAt       time.Time
		status      string
		attempts    int
		leaseOwner  string
		leaseUntil  any
		nextAttempt time.Time
		createdAt   time.Time
	}{
		{1, fixed.Add(-time.Minute), OutboxPending, 0, "", nil, fixed.Add(-time.Minute), fixed.Add(-10 * time.Minute)},
		{2, fixed.Add(-2 * time.Minute), OutboxFailed, 3, "worker-a", fixed.Add(time.Minute), fixed.Add(-time.Minute), fixed.Add(-20 * time.Minute)},
		{3, fixed.Add(time.Minute), OutboxPublished, 1, "", nil, fixed.Add(-time.Minute), fixed.Add(-3 * time.Minute)},
		{4, fixed.Add(-time.Minute), OutboxCompleted, 2, "", nil, fixed.Add(-time.Minute), fixed.Add(-4 * time.Minute)},
		{5, fixed.Add(-3 * time.Minute), OutboxFailed, 2, "", nil, fixed.Add(-time.Second), fixed.Add(-5 * time.Minute)},
	}
	for _, row := range outboxRows {
		if err := db.Exec(`
			INSERT INTO order_timeout_outbox_v2
				(order_id, due_at, status, attempts, last_error, lease_owner, lease_until, next_attempt_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?)
		`, row.orderID, row.dueAt, row.status, row.attempts, row.leaseOwner, row.leaseUntil, row.nextAttempt, row.createdAt, row.createdAt).Error; err != nil {
			t.Fatalf("insert outbox row %d: %v", row.orderID, err)
		}
	}

	orderRows := []struct {
		status    string
		updatedAt time.Time
	}{
		{OrderStatusReserving, fixed.Add(-10 * time.Minute)},
		{OrderStatusPending, fixed.Add(-20 * time.Minute)},
		{OrderStatusPaying, fixed.Add(-6 * time.Minute)},
		{OrderStatusPaid, fixed.Add(-time.Minute)},
		{OrderStatusCancelling, fixed.Add(-time.Minute)},
		{OrderStatusCancelled, fixed.Add(-time.Minute)},
		{OrderStatusFinished, fixed.Add(-time.Minute)},
		{OrderStatusFailed, fixed.Add(-time.Minute)},
		{OrderStatusReconciliationRequired, fixed.Add(-30 * time.Minute)},
		{OrderStatusReconciliationRequired, fixed.Add(-2 * time.Minute)},
	}
	for index, row := range orderRows {
		if err := db.Exec(`INSERT INTO orders_v2 (id, status, updated_at) VALUES (?, ?, ?)`, index+1, row.status, row.updatedAt).Error; err != nil {
			t.Fatalf("insert order row %d: %v", index+1, err)
		}
	}

	counter := &queryCountingLogger{delegate: gormlogger.Default.LogMode(gormlogger.Silent)}
	reporter, err := NewReliabilityReporter(db.Session(&gorm.Session{Logger: counter}), 5*time.Minute)
	if err != nil {
		t.Fatalf("create reliability reporter: %v", err)
	}
	reporter.now = func() time.Time { return fixed }

	snapshot, err := reporter.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("collect snapshot: %v", err)
	}
	if counter.Count() != 2 {
		t.Fatalf("expected exactly two aggregate queries, got %d", counter.Count())
	}

	if snapshot.CollectedAt != fixed {
		t.Fatalf("unexpected collection time: %s", snapshot.CollectedAt)
	}
	if snapshot.Outbox.ByStatus.Pending != 1 || snapshot.Outbox.ByStatus.Failed != 2 ||
		snapshot.Outbox.ByStatus.Published != 1 || snapshot.Outbox.ByStatus.Completed != 1 {
		t.Fatalf("unexpected outbox status counts: %+v", snapshot.Outbox.ByStatus)
	}
	if snapshot.Outbox.Leased != 1 || snapshot.Outbox.RetryReady != 2 || snapshot.Outbox.Overdue != 3 {
		t.Fatalf("unexpected outbox operational counts: %+v", snapshot.Outbox)
	}
	if snapshot.Outbox.OldestActionableAgeSeconds != 1200 {
		t.Fatalf("expected oldest actionable age 1200, got %d", snapshot.Outbox.OldestActionableAgeSeconds)
	}
	if snapshot.Outbox.MaximumAttempts != 3 || snapshot.Outbox.TotalFailedAttempts != 5 {
		t.Fatalf("unexpected attempt indicators: %+v", snapshot.Outbox)
	}

	if snapshot.Orders.ByStatus.Reserving != 1 || snapshot.Orders.ByStatus.Pending != 1 ||
		snapshot.Orders.ByStatus.Paying != 1 || snapshot.Orders.ByStatus.Paid != 1 ||
		snapshot.Orders.ByStatus.Cancelling != 1 || snapshot.Orders.ByStatus.Cancelled != 1 ||
		snapshot.Orders.ByStatus.Finished != 1 || snapshot.Orders.ByStatus.Failed != 1 ||
		snapshot.Orders.ByStatus.ReconciliationRequired != 2 {
		t.Fatalf("unexpected order status counts: %+v", snapshot.Orders.ByStatus)
	}
	if snapshot.Orders.ReconciliationRequired != 2 || snapshot.Orders.OldestReconciliationAgeSeconds != 1800 {
		t.Fatalf("unexpected reconciliation indicators: %+v", snapshot.Orders)
	}
	if snapshot.Orders.StuckTransient != 2 || snapshot.Orders.TransientStuckThresholdSeconds != 300 {
		t.Fatalf("unexpected transient indicators: %+v", snapshot.Orders)
	}
}

func TestReliabilityReporterEmptyDatabase(t *testing.T) {
	db := openOutboxLeaseTestDB(t)
	createReliabilityOrdersTable(t, db)
	fixed := time.Date(2026, time.July, 11, 12, 0, 0, 0, time.UTC)
	reporter, err := NewReliabilityReporter(db, 5*time.Minute)
	if err != nil {
		t.Fatalf("create reliability reporter: %v", err)
	}
	reporter.now = func() time.Time { return fixed }

	snapshot, err := reporter.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("collect empty snapshot: %v", err)
	}
	if snapshot.Outbox.ByStatus != (OutboxStatusIndicators{}) || snapshot.Orders.ByStatus != (OrderStatusIndicators{}) {
		t.Fatalf("empty database returned status counts: outbox=%+v orders=%+v", snapshot.Outbox.ByStatus, snapshot.Orders.ByStatus)
	}
	if snapshot.Outbox.OldestActionableAgeSeconds != 0 || snapshot.Orders.OldestReconciliationAgeSeconds != 0 {
		t.Fatalf("empty database returned non-zero ages: %+v", snapshot)
	}
}

func createReliabilityOrdersTable(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.Exec(`
		CREATE TABLE orders_v2 (
			id BIGINT NOT NULL,
			status VARCHAR(32) NOT NULL,
			updated_at DATETIME(3) NOT NULL,
			PRIMARY KEY (id),
			KEY idx_orders_status_updated (status, updated_at)
		) ENGINE=InnoDB
	`).Error; err != nil {
		t.Fatalf("create reliability orders table: %v", err)
	}
}

type queryCountingLogger struct {
	delegate gormlogger.Interface
	mu       sync.Mutex
	count    int
}

func (logger *queryCountingLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	logger.delegate = logger.delegate.LogMode(level)
	return logger
}

func (logger *queryCountingLogger) Info(ctx context.Context, message string, values ...any) {
	logger.delegate.Info(ctx, message, values...)
}

func (logger *queryCountingLogger) Warn(ctx context.Context, message string, values ...any) {
	logger.delegate.Warn(ctx, message, values...)
}

func (logger *queryCountingLogger) Error(ctx context.Context, message string, values ...any) {
	logger.delegate.Error(ctx, message, values...)
}

func (logger *queryCountingLogger) Trace(ctx context.Context, started time.Time, query func() (string, int64), err error) {
	logger.mu.Lock()
	logger.count++
	logger.mu.Unlock()
	logger.delegate.Trace(ctx, started, query, err)
}

func (logger *queryCountingLogger) Count() int {
	logger.mu.Lock()
	defer logger.mu.Unlock()
	return logger.count
}

var _ gormlogger.Interface = (*queryCountingLogger)(nil)
