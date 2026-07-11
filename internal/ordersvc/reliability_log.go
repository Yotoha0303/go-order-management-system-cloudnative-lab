package ordersvc

import (
	"context"
	"log/slog"
	"time"
)

func RunReliabilityLogLoop(
	ctx context.Context,
	snapshotter reliabilitySnapshotter,
	interval time.Duration,
	logger *slog.Logger,
) {
	if snapshotter == nil {
		return
	}
	if interval <= 0 {
		interval = time.Minute
	}
	if logger == nil {
		logger = slog.Default()
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			snapshot, err := snapshotter.Snapshot(ctx)
			if err != nil {
				logger.Warn("collect order reliability indicators", "error", err)
				continue
			}
			logger.Info(
				"order reliability indicators",
				"collected_at", snapshot.CollectedAt,
				"query_duration_ms", snapshot.QueryDurationMS,
				"outbox_pending", snapshot.Outbox.ByStatus.Pending,
				"outbox_failed", snapshot.Outbox.ByStatus.Failed,
				"outbox_published", snapshot.Outbox.ByStatus.Published,
				"outbox_completed", snapshot.Outbox.ByStatus.Completed,
				"outbox_leased", snapshot.Outbox.Leased,
				"outbox_retry_ready", snapshot.Outbox.RetryReady,
				"outbox_overdue", snapshot.Outbox.Overdue,
				"outbox_oldest_actionable_age_seconds", snapshot.Outbox.OldestActionableAgeSeconds,
				"outbox_maximum_attempts", snapshot.Outbox.MaximumAttempts,
				"outbox_total_failed_attempts", snapshot.Outbox.TotalFailedAttempts,
				"orders_reconciliation_required", snapshot.Orders.ReconciliationRequired,
				"orders_oldest_reconciliation_age_seconds", snapshot.Orders.OldestReconciliationAgeSeconds,
				"orders_stuck_transient", snapshot.Orders.StuckTransient,
				"orders_transient_stuck_threshold_seconds", snapshot.Orders.TransientStuckThresholdSeconds,
			)
		}
	}
}
