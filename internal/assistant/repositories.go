package assistant

import (
	"context"
	"time"
)

type InventoryQueryRepository interface {
	ListLowStockProducts(ctx context.Context, threshold, limit int) ([]LowStockProduct, error)
}

type OrderQueryRepository interface {
	SummarizeOrderStatus(ctx context.Context, from, to time.Time) ([]OrderStatusCount, error)
}

type CallLogRepository interface {
	Save(ctx context.Context, log AICallLog) error
}
