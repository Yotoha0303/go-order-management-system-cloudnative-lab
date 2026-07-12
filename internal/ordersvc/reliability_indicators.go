package ordersvc

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"gorm.io/gorm"
)

type OutboxStatusIndicators struct {
	Pending   int64 `json:"pending"`
	Failed    int64 `json:"failed"`
	Published int64 `json:"published"`
	Completed int64 `json:"completed"`
}

type OutboxIndicators struct {
	ByStatus                   OutboxStatusIndicators `json:"by_status"`
	Leased                     int64                  `json:"leased"`
	RetryReady                 int64                  `json:"retry_ready"`
	Overdue                    int64                  `json:"overdue"`
	OldestActionableAgeSeconds int64                  `json:"oldest_actionable_age_seconds"`
	MaximumAttempts            int64                  `json:"maximum_attempts"`
	TotalFailedAttempts        int64                  `json:"total_failed_attempts"`
}

type OrderStatusIndicators struct {
	Reserving              int64 `json:"reserving"`
	Pending                int64 `json:"pending"`
	Paying                 int64 `json:"paying"`
	Paid                   int64 `json:"paid"`
	Cancelling             int64 `json:"cancelling"`
	Cancelled              int64 `json:"cancelled"`
	Finished               int64 `json:"finished"`
	Failed                 int64 `json:"failed"`
	ReconciliationRequired int64 `json:"reconciliation_required"`
}

type OrderSagaIndicators struct {
	ByStatus                       OrderStatusIndicators `json:"by_status"`
	ReconciliationRequired         int64                 `json:"reconciliation_required"`
	OldestReconciliationAgeSeconds int64                 `json:"oldest_reconciliation_age_seconds"`
	StuckTransient                 int64                 `json:"stuck_transient"`
	TransientStuckThresholdSeconds int64                 `json:"transient_stuck_threshold_seconds"`
}

type ReliabilitySnapshot struct {
	CollectedAt     time.Time           `json:"collected_at"`
	QueryDurationMS int64               `json:"query_duration_ms"`
	Outbox          OutboxIndicators    `json:"outbox"`
	Orders          OrderSagaIndicators `json:"orders"`
}

type reliabilitySnapshotter interface {
	Snapshot(context.Context) (ReliabilitySnapshot, error)
}

type ReliabilityReporter struct {
	db             *gorm.DB
	stuckThreshold time.Duration
	now            func() time.Time
}

func NewReliabilityReporter(db *gorm.DB, stuckThreshold time.Duration) (*ReliabilityReporter, error) {
	if db == nil {
		return nil, errors.New("reliability reporter database is required")
	}
	if stuckThreshold <= 0 {
		stuckThreshold = 5 * time.Minute
	}
	return &ReliabilityReporter{
		db:             db,
		stuckThreshold: stuckThreshold,
		now:            time.Now,
	}, nil
}

func (reporter *ReliabilityReporter) Snapshot(ctx context.Context) (ReliabilitySnapshot, error) {
	if reporter == nil || reporter.db == nil {
		return ReliabilitySnapshot{}, errors.New("reliability reporter is not configured")
	}
	collectedAt := reporter.now().UTC()
	started := time.Now()

	var outboxRow struct {
		Pending          int64
		Failed           int64
		Published        int64
		Completed        int64
		Leased           int64
		RetryReady       int64
		Overdue          int64
		OldestActionable sql.NullTime
		MaximumAttempts  int64
		FailedAttempts   int64
	}
	if err := reporter.db.WithContext(ctx).Raw(`
		SELECT
			COALESCE(SUM(status = ?), 0) AS pending,
			COALESCE(SUM(status = ?), 0) AS failed,
			COALESCE(SUM(status = ?), 0) AS published,
			COALESCE(SUM(status = ?), 0) AS completed,
			COALESCE(SUM(lease_owner <> '' AND lease_until IS NOT NULL AND lease_until > ?), 0) AS leased,
			COALESCE(SUM(status IN (?, ?) AND next_attempt_at <= ? AND (lease_until IS NULL OR lease_until < ?)), 0) AS retry_ready,
			COALESCE(SUM(due_at < ? AND status <> ?), 0) AS overdue,
			MIN(CASE WHEN status IN (?, ?) THEN created_at END) AS oldest_actionable,
			COALESCE(MAX(attempts), 0) AS maximum_attempts,
			COALESCE(SUM(CASE WHEN status = ? THEN attempts ELSE 0 END), 0) AS failed_attempts
		FROM order_timeout_outbox_v2
	`,
		OutboxPending,
		OutboxFailed,
		OutboxPublished,
		OutboxCompleted,
		collectedAt,
		OutboxPending,
		OutboxFailed,
		collectedAt,
		collectedAt,
		collectedAt,
		OutboxCompleted,
		OutboxPending,
		OutboxFailed,
		OutboxFailed,
	).Scan(&outboxRow).Error; err != nil {
		return ReliabilitySnapshot{}, err
	}

	var orderRow struct {
		Reserving              int64
		Pending                int64
		Paying                 int64
		Paid                   int64
		Cancelling             int64
		Cancelled              int64
		Finished               int64
		Failed                 int64
		ReconciliationRequired int64
		OldestReconciliation   sql.NullTime
		StuckTransient         int64
	}
	stuckBefore := collectedAt.Add(-reporter.stuckThreshold)
	if err := reporter.db.WithContext(ctx).Raw(`
		SELECT
			COALESCE(SUM(status = ?), 0) AS reserving,
			COALESCE(SUM(status = ?), 0) AS pending,
			COALESCE(SUM(status = ?), 0) AS paying,
			COALESCE(SUM(status = ?), 0) AS paid,
			COALESCE(SUM(status = ?), 0) AS cancelling,
			COALESCE(SUM(status = ?), 0) AS cancelled,
			COALESCE(SUM(status = ?), 0) AS finished,
			COALESCE(SUM(status = ?), 0) AS failed,
			COALESCE(SUM(status = ?), 0) AS reconciliation_required,
			MIN(CASE WHEN status = ? THEN updated_at END) AS oldest_reconciliation,
			COALESCE(SUM(status IN (?, ?, ?) AND updated_at < ?), 0) AS stuck_transient
		FROM orders_v2
	`,
		OrderStatusReserving,
		OrderStatusPending,
		OrderStatusPaying,
		OrderStatusPaid,
		OrderStatusCancelling,
		OrderStatusCancelled,
		OrderStatusFinished,
		OrderStatusFailed,
		OrderStatusReconciliationRequired,
		OrderStatusReconciliationRequired,
		OrderStatusReserving,
		OrderStatusPaying,
		OrderStatusCancelling,
		stuckBefore,
	).Scan(&orderRow).Error; err != nil {
		return ReliabilitySnapshot{}, err
	}

	return ReliabilitySnapshot{
		CollectedAt:     collectedAt,
		QueryDurationMS: time.Since(started).Milliseconds(),
		Outbox: OutboxIndicators{
			ByStatus: OutboxStatusIndicators{
				Pending:   outboxRow.Pending,
				Failed:    outboxRow.Failed,
				Published: outboxRow.Published,
				Completed: outboxRow.Completed,
			},
			Leased:                     outboxRow.Leased,
			RetryReady:                 outboxRow.RetryReady,
			Overdue:                    outboxRow.Overdue,
			OldestActionableAgeSeconds: ageSeconds(collectedAt, outboxRow.OldestActionable),
			MaximumAttempts:            outboxRow.MaximumAttempts,
			TotalFailedAttempts:        outboxRow.FailedAttempts,
		},
		Orders: OrderSagaIndicators{
			ByStatus: OrderStatusIndicators{
				Reserving:              orderRow.Reserving,
				Pending:                orderRow.Pending,
				Paying:                 orderRow.Paying,
				Paid:                   orderRow.Paid,
				Cancelling:             orderRow.Cancelling,
				Cancelled:              orderRow.Cancelled,
				Finished:               orderRow.Finished,
				Failed:                 orderRow.Failed,
				ReconciliationRequired: orderRow.ReconciliationRequired,
			},
			ReconciliationRequired:         orderRow.ReconciliationRequired,
			OldestReconciliationAgeSeconds: ageSeconds(collectedAt, orderRow.OldestReconciliation),
			StuckTransient:                 orderRow.StuckTransient,
			TransientStuckThresholdSeconds: int64(reporter.stuckThreshold.Seconds()),
		},
	}, nil
}

func ageSeconds(now time.Time, value sql.NullTime) int64 {
	if !value.Valid || value.Time.After(now) {
		return 0
	}
	return int64(now.Sub(value.Time).Seconds())
}
