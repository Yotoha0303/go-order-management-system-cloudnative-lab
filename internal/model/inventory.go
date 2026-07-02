package model

import "time"

type Inventory struct {
	ID            int64     `gorm:"primaryKey;autoIncrement;type:bigint" json:"id"`
	ProductID     int64     `gorm:"type:bigint;not null;uniqueIndex:uk_inventory_product_id" json:"product_id"`
	StockQuantity int64     `gorm:"type:bigint;not null;default:0;check:stock_quantity >= 0" json:"stock_quantity"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (Inventory) TableName() string {
	return "product_inventories"
}
