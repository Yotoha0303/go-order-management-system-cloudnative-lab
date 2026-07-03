package dao

import (
	"context"
	"go-order-management-system/internal/model"
	"time"

	"gorm.io/gorm"
)

func CreateOrder(ctx context.Context, db *gorm.DB, order *model.Order) error {
	return db.WithContext(ctx).Create(order).Error
}

func CreateOrderItem(ctx context.Context, db *gorm.DB, items *model.OrderItem) error {
	return db.WithContext(ctx).Create(items).Error
}

func GetOrderByID(ctx context.Context, db *gorm.DB, userID, id int64) (*model.Order, error) {
	var order model.Order
	return &order, db.WithContext(ctx).Model(&order).Where("user_id = ? AND id = ?", userID, id).First(&order).Error
}

func ListOrders(ctx context.Context, db *gorm.DB, userID int64, pageSize, offset int) ([]*model.Order, int64, error) {
	var orders []*model.Order
	query := db.WithContext(ctx).Model(&model.Order{}).Where("user_id = ?", userID)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := query.Order("id DESC").Limit(pageSize).Offset(offset).Find(&orders).Error; err != nil {
		return nil, 0, err
	}

	return orders, total, nil
}

func ListOrderItemsByOrderID(ctx context.Context, db *gorm.DB, orderID int64) ([]*model.OrderItem, error) {
	var items []*model.OrderItem
	return items, db.WithContext(ctx).Model(&model.OrderItem{}).Where("order_id = ?", orderID).Order("id ASC").Find(&items).Error
}

func PatchOrderStatus(ctx context.Context, db *gorm.DB, userID, orderID int64, fromStatus int8, toStatus int8, timeField string) (int64, error) {
	result := db.WithContext(ctx).Model(&model.Order{}).Where("user_id = ? AND id = ? AND status = ?", userID, orderID, fromStatus).Updates(
		map[string]interface{}{
			"status":  toStatus,
			timeField: time.Now(),
		})
	return result.RowsAffected, result.Error
}

func PatchOrderTotalPriceFen(ctx context.Context, db *gorm.DB, orderID int64, totalPriceFen int64, userID int64) error {
	return db.WithContext(ctx).Model(&model.Order{}).Where("id = ? AND user_id = ?", orderID, userID).Update("total_amount_fen", totalPriceFen).Error
}
