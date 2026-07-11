package ordersvc

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

const (
	ReconciliationPlanReady         = "ready"
	ReconciliationPlanAlreadyFinal  = "already_final"
	ReconciliationPlanUnsupported   = "unsupported_action"
	ReconciliationPlanStateMismatch = "state_mismatch"
	ReconciliationPlanOrderMissing  = "order_missing"
)

type ReconciliationDryRunItem struct {
	TaskID              uint64 `json:"task_id"`
	OrderID             int64  `json:"order_id"`
	Action              string `json:"action"`
	TaskStatus          string `json:"task_status"`
	CurrentOrderStatus  string `json:"current_order_status,omitempty"`
	IntendedTargetState string `json:"intended_target_state,omitempty"`
	Classification      string `json:"classification"`
	Reason              string `json:"reason,omitempty"`
}

type ReconciliationDryRunReport struct {
	GeneratedAt   time.Time                  `json:"generated_at"`
	EligibleCount int                        `json:"eligible_count"`
	Items         []ReconciliationDryRunItem `json:"items"`
}

type reconciliationDryRunRow struct {
	TaskID      uint64
	OrderID     int64
	Action      string
	TaskStatus  string
	OrderStatus sql.NullString
}

func (worker *ReconciliationWorker) DryRun(ctx context.Context) (ReconciliationDryRunReport, error) {
	if worker == nil || worker.db == nil {
		return ReconciliationDryRunReport{}, errors.New("reconciliation worker is not configured")
	}
	if err := ctx.Err(); err != nil {
		return ReconciliationDryRunReport{}, err
	}

	now := worker.now().UTC()
	var rows []reconciliationDryRunRow
	if err := worker.db.WithContext(ctx).Raw(`
		SELECT
			t.id AS task_id,
			t.order_id,
			t.action,
			t.status AS task_status,
			o.status AS order_status
		FROM order_reconciliation_tasks AS t
		LEFT JOIN orders_v2 AS o ON o.id = t.order_id
		WHERE t.status IN (?, ?)
		  AND t.next_attempt_at <= ?
		  AND (t.lease_until IS NULL OR t.lease_until < ?)
		ORDER BY t.id ASC
		LIMIT ?
	`,
		ReconciliationTaskPending,
		ReconciliationTaskFailed,
		now,
		now,
		worker.cfg.BatchSize,
	).Scan(&rows).Error; err != nil {
		return ReconciliationDryRunReport{}, err
	}

	items := make([]ReconciliationDryRunItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, planReconciliation(row))
	}
	return ReconciliationDryRunReport{
		GeneratedAt:   now,
		EligibleCount: len(items),
		Items:         items,
	}, nil
}

func planReconciliation(row reconciliationDryRunRow) ReconciliationDryRunItem {
	item := ReconciliationDryRunItem{
		TaskID:         row.TaskID,
		OrderID:        row.OrderID,
		Action:         row.Action,
		TaskStatus:     row.TaskStatus,
		Classification: ReconciliationPlanReady,
	}

	target, supported := reconciliationTargetState(row.Action)
	if !supported {
		item.Classification = ReconciliationPlanUnsupported
		item.Reason = ErrUnsupportedReconciliationAction.Error()
		if row.OrderStatus.Valid {
			item.CurrentOrderStatus = row.OrderStatus.String
		}
		return item
	}
	item.IntendedTargetState = target

	if !row.OrderStatus.Valid {
		item.Classification = ReconciliationPlanOrderMissing
		item.Reason = "order not found"
		return item
	}
	item.CurrentOrderStatus = row.OrderStatus.String

	switch row.OrderStatus.String {
	case OrderStatusReconciliationRequired:
		return item
	case target:
		item.Classification = ReconciliationPlanAlreadyFinal
		item.Reason = "order already has the intended target state"
		return item
	default:
		item.Classification = ReconciliationPlanStateMismatch
		item.Reason = ErrReconciliationStateMismatch.Error()
		return item
	}
}

func reconciliationTargetState(action string) (string, bool) {
	switch action {
	case ReconciliationActionReleaseInventoryAndFail:
		return OrderStatusFailed, true
	case ReconciliationActionFinalizeCancel:
		return OrderStatusCancelled, true
	case ReconciliationActionFinalizePayment:
		return OrderStatusPaid, true
	default:
		return "", false
	}
}
