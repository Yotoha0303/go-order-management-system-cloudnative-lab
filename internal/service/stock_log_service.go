package service

import (
	"context"
	"go-order-management-system-cloudnative-lab/internal/dao"
	"go-order-management-system-cloudnative-lab/internal/model"
)

func (p *StockLogService) CreateStockLog(ctx context.Context, log *model.StockLog) error {
	return dao.CreateStockLog(ctx, p.db, log)
}

func (p *StockLogService) ListStockLogsByProductID(ctx context.Context, productID *int64) ([]*model.StockLog, error) {
	return dao.ListStockLogsByProductID(ctx, p.db, productID)
}
