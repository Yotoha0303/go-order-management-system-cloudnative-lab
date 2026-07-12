package ordersvc

import (
	"context"

	platformmetrics "go-order-management-system/internal/platform/metrics"
)

func ReliabilityPrometheusCollector(snapshotter reliabilitySnapshotter) platformmetrics.Collector {
	return platformmetrics.Collector{
		Name: "order_reliability",
		Collect: func(ctx context.Context, registry *platformmetrics.Registry) error {
			snapshot, err := snapshotter.Snapshot(ctx)
			if err != nil {
				return err
			}
			setOutboxMetrics(registry, snapshot.Outbox)
			setOrderMetrics(registry, snapshot.Orders)
			registry.SetGauge(
				"go_order_reliability_snapshot_query_duration_seconds",
				"Duration of the latest Order reliability snapshot query in seconds.",
				nil,
				float64(snapshot.QueryDurationMS)/1000,
			)
			registry.SetGauge(
				"go_order_reliability_snapshot_collected_timestamp_seconds",
				"Unix timestamp of the latest successful Order reliability snapshot.",
				nil,
				float64(snapshot.CollectedAt.Unix()),
			)
			return nil
		},
	}
}

func setOutboxMetrics(registry *platformmetrics.Registry, indicators OutboxIndicators) {
	statuses := map[string]int64{
		"pending":   indicators.ByStatus.Pending,
		"failed":    indicators.ByStatus.Failed,
		"published": indicators.ByStatus.Published,
		"completed": indicators.ByStatus.Completed,
	}
	for status, value := range statuses {
		registry.SetGauge(
			"go_order_outbox_events",
			"Current timeout Outbox event count by bounded status.",
			platformmetrics.Labels{"status": status},
			float64(value),
		)
	}
	registry.SetGauge("go_order_outbox_leased", "Current timeout Outbox events with an active lease.", nil, float64(indicators.Leased))
	registry.SetGauge("go_order_outbox_retry_ready", "Current timeout Outbox events ready for retry.", nil, float64(indicators.RetryReady))
	registry.SetGauge("go_order_outbox_overdue", "Current timeout Outbox events overdue and not completed.", nil, float64(indicators.Overdue))
	registry.SetGauge("go_order_outbox_oldest_actionable_age_seconds", "Age in seconds of the oldest actionable timeout Outbox event.", nil, float64(indicators.OldestActionableAgeSeconds))
	registry.SetGauge("go_order_outbox_maximum_attempts", "Maximum publish attempt count among timeout Outbox events.", nil, float64(indicators.MaximumAttempts))
	registry.SetGauge("go_order_outbox_failed_attempts_total_snapshot", "Snapshot total of failed publish attempts recorded on failed timeout Outbox events.", nil, float64(indicators.TotalFailedAttempts))
}

func setOrderMetrics(registry *platformmetrics.Registry, indicators OrderSagaIndicators) {
	statuses := map[string]int64{
		"reserving":               indicators.ByStatus.Reserving,
		"pending":                 indicators.ByStatus.Pending,
		"paying":                  indicators.ByStatus.Paying,
		"paid":                    indicators.ByStatus.Paid,
		"cancelling":              indicators.ByStatus.Cancelling,
		"cancelled":               indicators.ByStatus.Cancelled,
		"finished":                indicators.ByStatus.Finished,
		"failed":                  indicators.ByStatus.Failed,
		"reconciliation_required": indicators.ByStatus.ReconciliationRequired,
	}
	for status, value := range statuses {
		registry.SetGauge(
			"go_order_orders",
			"Current Order count by bounded Saga status.",
			platformmetrics.Labels{"status": status},
			float64(value),
		)
	}
	registry.SetGauge("go_order_reconciliation_required", "Current Orders requiring reconciliation.", nil, float64(indicators.ReconciliationRequired))
	registry.SetGauge("go_order_reconciliation_oldest_age_seconds", "Age in seconds of the oldest Order requiring reconciliation.", nil, float64(indicators.OldestReconciliationAgeSeconds))
	registry.SetGauge("go_order_saga_stuck_transient", "Current Orders stuck in a transient Saga status beyond the configured threshold.", nil, float64(indicators.StuckTransient))
	registry.SetGauge("go_order_saga_stuck_threshold_seconds", "Configured threshold in seconds for a transient Saga status to be considered stuck.", nil, float64(indicators.TransientStuckThresholdSeconds))
}
