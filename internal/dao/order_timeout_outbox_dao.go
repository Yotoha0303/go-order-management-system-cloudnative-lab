package dao

import (
	"context"
	"time"

	"go-order-management-system-cloudnative-lab/internal/model"

	"gorm.io/gorm"
)

func CreateOrderTimeoutOutbox(ctx context.Context, db *gorm.DB, outbox *model.OrderTimeoutOutbox) error {
	return db.WithContext(ctx).Create(outbox).Error
}

func GetOrderTimeoutOutbox(
	ctx context.Context,
	db *gorm.DB,
	eventID string,
	orderID int64,
) (*model.OrderTimeoutOutbox, error) {
	var outbox model.OrderTimeoutOutbox
	err := db.WithContext(ctx).
		Where("event_id = ? AND order_id = ?", eventID, orderID).
		First(&outbox).Error
	return &outbox, err
}

func ListPendingOrderTimeoutOutbox(
	ctx context.Context,
	db *gorm.DB,
	now time.Time,
	limit int,
) ([]model.OrderTimeoutOutbox, error) {
	var events []model.OrderTimeoutOutbox
	err := db.WithContext(ctx).
		Where("published_at IS NULL AND next_attempt_at <= ?", now).
		Order("id ASC").
		Limit(limit).
		Find(&events).Error
	return events, err
}

func MarkOrderTimeoutOutboxPublished(ctx context.Context, db *gorm.DB, eventID string, now time.Time) error {
	return db.WithContext(ctx).
		Model(&model.OrderTimeoutOutbox{}).
		Where("event_id = ? AND published_at IS NULL", eventID).
		Updates(map[string]any{
			"published_at": now,
			"last_error":   "",
		}).Error
}

func MarkOrderTimeoutOutboxFailed(
	ctx context.Context,
	db *gorm.DB,
	eventID, lastError string,
	nextAttemptAt time.Time,
) error {
	return db.WithContext(ctx).
		Model(&model.OrderTimeoutOutbox{}).
		Where("event_id = ? AND published_at IS NULL", eventID).
		Updates(map[string]any{
			"attempts":        gorm.Expr("attempts + 1"),
			"last_error":      lastError,
			"next_attempt_at": nextAttemptAt,
		}).Error
}
