package assistantadapter

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go-order-management-system/internal/assistant"
	"go-order-management-system/internal/model"

	"gorm.io/gorm"
)

type MySQLRepository struct {
	db *gorm.DB
}

var (
	_ assistant.InventoryQueryRepository = (*MySQLRepository)(nil)
	_ assistant.OrderQueryRepository     = (*MySQLRepository)(nil)
	_ assistant.CallLogRepository        = (*MySQLRepository)(nil)
)

func NewMySQLRepository(db *gorm.DB) (*MySQLRepository, error) {
	if db == nil {
		return nil, errors.New("create assistant MySQL repository: database must not be nil")
	}
	return &MySQLRepository{db: db}, nil
}

func (r *MySQLRepository) ListLowStockProducts(
	ctx context.Context,
	threshold, limit int,
) ([]assistant.LowStockProduct, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if threshold < 0 || limit < 1 {
		return nil, errors.New("invalid low-stock query parameters")
	}

	var rows []struct {
		ProductID int64
		Name      string
		Stock     int64
	}
	err := r.db.WithContext(ctx).
		Table("product_inventories AS inventory").
		Select("product.id AS product_id, product.name, inventory.stock_quantity AS stock").
		Joins("INNER JOIN products AS product ON product.id = inventory.product_id").
		Where("inventory.stock_quantity <= ?", threshold).
		Order("inventory.stock_quantity ASC").
		Order("product.id ASC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	products := make([]assistant.LowStockProduct, 0, len(rows))
	for _, row := range rows {
		products = append(products, assistant.LowStockProduct{
			ProductID: row.ProductID,
			Name:      row.Name,
			Stock:     row.Stock,
		})
	}
	return products, nil
}

func (r *MySQLRepository) SummarizeOrderStatus(
	ctx context.Context,
	from, to time.Time,
) ([]assistant.OrderStatusCount, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !from.Before(to) {
		return nil, errors.New("from must be before to")
	}

	var rows []struct {
		Status int8
		Count  int64
	}
	if err := r.db.WithContext(ctx).
		Model(&model.Order{}).
		Select("status, COUNT(*) AS count").
		Where("created_at >= ? AND created_at < ?", from, to).
		Group("status").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	counts := make([]assistant.OrderStatusCount, 0, len(rows))
	for _, row := range rows {
		status, err := mapOrderStatus(row.Status)
		if err != nil {
			return nil, err
		}
		counts = append(counts, assistant.OrderStatusCount{Status: status, Count: row.Count})
	}
	return counts, nil
}

func (r *MySQLRepository) Save(ctx context.Context, call assistant.AICallLog) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	log := &model.AICallLog{
		RequestID:        call.RequestID,
		UserID:           call.UserID,
		Intent:           string(call.Intent),
		ToolName:         call.ToolName,
		Provider:         call.Provider,
		Model:            call.Model,
		PromptTokens:     call.PromptTokens,
		CompletionTokens: call.CompletionTokens,
		TotalTokens:      call.TotalTokens,
		LatencyMS:        call.LatencyMS,
		Status:           string(call.Status),
		ErrorCode:        string(call.ErrorCode),
		CreatedAt:        call.CreatedAt,
	}
	return r.db.WithContext(ctx).Create(log).Error
}

func mapOrderStatus(status int8) (assistant.OrderStatus, error) {
	switch status {
	case model.OrderStatusPending:
		return assistant.OrderStatusPending, nil
	case model.OrderStatusPaid:
		return assistant.OrderStatusPaid, nil
	case model.OrderStatusFinished:
		return assistant.OrderStatusFinished, nil
	case model.OrderStatusCancelled:
		return assistant.OrderStatusCancelled, nil
	default:
		return "", fmt.Errorf("unsupported order status %d", status)
	}
}
