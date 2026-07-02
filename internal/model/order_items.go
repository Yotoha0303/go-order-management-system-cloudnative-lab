package model

import "time"

type OrderItem struct {
	ID              int64     `gorm:"primaryKey;autoIncrement;type:bigint" json:"id"`
	OrderID         int64     `gorm:"type:bigint;not null;index:idx_order_items_order_id" json:"order_id"`
	ProductID       int64     `gorm:"type:bigint;not null;index:idx_order_items_product_id" json:"product_id"`
	ProductName     string    `gorm:"type:varchar(100);not null" json:"product_name"`
	ProductPriceFen int64     `gorm:"column:product_price_fen;type:bigint;not null" json:"product_price_fen"`
	Quantity        int64     `gorm:"type:bigint;not null" json:"quantity"`
	SubtotalFen     int64     `gorm:"column:subtotal_fen;type:bigint;not null" json:"subtotal_fen"`
	CreatedAt       time.Time `gorm:"type:datetime;not null;default:CURRENT_TIMESTAMP" json:"created_at"`
}

func (OrderItem) TableName() string {
	return "order_items"
}
