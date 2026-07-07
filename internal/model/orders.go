package model

import "time"

const (
	OrderStatusPending   int8 = 1
	OrderStatusPaid      int8 = 2
	OrderStatusFinished  int8 = 3
	OrderStatusCancelled int8 = 4
)

type Order struct {
	ID             int64      `gorm:"primaryKey;autoIncrement;type:bigint" json:"id"`
	UserID         int64      `gorm:"type:bigint;not null;index:idx_orders_user_id_created_at,priority:1" json:"user_id"`
	OrderNo        string     `gorm:"type:varchar(255);not null;uniqueIndex:uk_orders_order_no" json:"order_no"`
	TotalAmountFen int64      `gorm:"column:total_amount_fen;type:bigint;not null" json:"total_amount_fen"`
	Status         int8       `gorm:"type:tinyint;not null;default:1;index:idx_orders_status" json:"status"`
	PaidAt         *time.Time `gorm:"type:datetime" json:"paid_at,omitempty"`
	CompletedAt    *time.Time `gorm:"type:datetime" json:"completed_at,omitempty"`
	CancelledAt    *time.Time `gorm:"type:datetime" json:"cancelled_at,omitempty"`
	CreatedAt      time.Time  `gorm:"type:datetime;not null;default:CURRENT_TIMESTAMP;index:idx_orders_user_id_created_at,priority:2" json:"created_at"`
	UpdatedAt      time.Time  `gorm:"type:datetime;not null;default:CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP" json:"updated_at"`
}

func (Order) TableName() string {
	return "orders"
}
