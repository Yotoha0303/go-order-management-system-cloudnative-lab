package model

import "time"

const (
	OrderBeingCreated   int8 = 1
	OrderAlreadyCreated int8 = 2
)

type OrderIdempotencyKey struct {
	ID             int64  `gorm:"primaryKey;autoIncrement;type:bigint" json:"id"`
	UserID         int64  `gorm:"type:bigint;not null;uniqueIndex:uk_user_id_idempotency_key,priority:1" json:"user_id"`
	IdempotencyKey string `gorm:"type:varchar(128);not null;uniqueIndex:uk_user_id_idempotency_key,priority:2" json:"idempotency_key"`
	RequestHash    string `gorm:"type:varchar(64);not null" json:"request_hash"`
	OrderID        *int64 `gorm:"type:bigint;index:idx_order_id" json:"order_id"`
	Status         int8   `gorm:"type:tinyint(2);not null;check:status IN (1,2)" json:"status"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (OrderIdempotencyKey) TableName() string {
	return "order_idempotency_keys"
}
