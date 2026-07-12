package ordersvc

import (
	"context"
	"strings"
	"testing"
	"time"

	platformmetrics "go-order-management-system/internal/platform/metrics"
)

type fixedReliabilitySnapshotter struct {
	snapshot ReliabilitySnapshot
	err      error
}

func (snapshotter fixedReliabilitySnapshotter) Snapshot(context.Context) (ReliabilitySnapshot, error) {
	return snapshotter.snapshot, snapshotter.err
}

func TestReliabilityPrometheusCollectorExportsBoundedGauges(t *testing.T) {
	registry := platformmetrics.NewRegistry()
	collector := ReliabilityPrometheusCollector(fixedReliabilitySnapshotter{snapshot: ReliabilitySnapshot{
		CollectedAt:     time.Unix(1_700_000_000, 0).UTC(),
		QueryDurationMS: 25,
		Outbox: OutboxIndicators{
			ByStatus:                   OutboxStatusIndicators{Pending: 2, Failed: 1, Published: 3, Completed: 4},
			Leased:                     1,
			RetryReady:                 2,
			Overdue:                    1,
			OldestActionableAgeSeconds: 12,
			MaximumAttempts:            5,
			TotalFailedAttempts:        7,
		},
		Orders: OrderSagaIndicators{
			ByStatus:                        OrderStatusIndicators{Pending: 4, Paid: 2, ReconciliationRequired: 1},
			ReconciliationRequired:         1,
			OldestReconciliationAgeSeconds: 30,
			StuckTransient:                 2,
			TransientStuckThresholdSeconds: 300,
		},
	}})

	if err := collector.Collect(context.Background(), registry); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}
	body := string(registry.Gather())
	for _, expected := range []string{
		`go_order_outbox_events{status="pending"} 2`,
		`go_order_outbox_events{status="failed"} 1`,
		`go_order_orders{status="pending"} 4`,
		`go_order_orders{status="reconciliation_required"} 1`,
		`go_order_reconciliation_required 1`,
		`go_order_saga_stuck_transient 2`,
		`go_order_reliability_snapshot_query_duration_seconds 0.025`,
		`go_order_reliability_snapshot_collected_timestamp_seconds 1.7e+09`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected %q in metrics:\n%s", expected, body)
		}
	}
}
