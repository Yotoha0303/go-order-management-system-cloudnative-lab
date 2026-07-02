package dao

import (
	"context"
	"go-order-management-system/internal/model"

	"gorm.io/gorm"
)

func CreateStockLog(ctx context.Context, db *gorm.DB, log *model.StockLog) error {
	return db.WithContext(ctx).Create(log).Error
}

func ListStockLogsByProductID(ctx context.Context, db *gorm.DB, productID *int64) ([]*model.StockLog, error) {
	var logs []*model.StockLog
	if productID == nil || *productID == 0 {
		return logs, db.WithContext(ctx).Order("created_at desc").Find(&logs).Error
	}
	return logs, db.WithContext(ctx).Where("product_id = ?", *productID).Order("created_at desc").Find(&logs).Error
}
