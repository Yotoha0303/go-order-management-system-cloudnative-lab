package dao

import (
	"context"
	"go-order-management-system/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TryCreateOrderIdempotencyKey(db *gorm.DB, ctx context.Context, userID int64, idempotencyKey, requestHash string) (bool, error) {
	orderIdempotencyKey := &model.OrderIdempotencyKey{
		UserID:         userID,
		IdempotencyKey: idempotencyKey,
		RequestHash:    requestHash,
		Status:         model.OrderBeingCreated,
	}
	result := db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(orderIdempotencyKey)
	return result.RowsAffected == 1, result.Error
}

func GetOrderIdempotencyKey(db *gorm.DB, ctx context.Context, userID int64, idempotencyKey string) (*model.OrderIdempotencyKey, error) {
	var orderIdempotencyKey model.OrderIdempotencyKey
	if err := db.WithContext(ctx).Where("user_id = ? AND idempotency_key = ?", userID, idempotencyKey).First(&orderIdempotencyKey).Error; err != nil {
		return nil, err
	}
	return &orderIdempotencyKey, nil
}

func CompleteOrderIdempotencyKey(db *gorm.DB, ctx context.Context, userID int64, idempotencyKey string, orderID int64) (int64, error) {
	result := db.WithContext(ctx).
		Model(&model.OrderIdempotencyKey{}).
		Where("user_id = ? AND idempotency_key = ? AND status = ?", userID, idempotencyKey, model.OrderBeingCreated).
		Updates(map[string]interface{}{
			"order_id": orderID,
			"status":   model.OrderAlreadyCreated,
		})
	return result.RowsAffected, result.Error
}
