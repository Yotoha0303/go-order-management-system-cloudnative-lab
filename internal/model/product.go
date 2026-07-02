package model

import "time"

const (
	ProductStatusOnSale  int8 = 1
	ProductStatusOffSale int8 = 2
)

type Product struct {
	ID          int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	Name        string `gorm:"type:varchar(100);not null" json:"name"`
	Description string `gorm:"type:varchar(500);not null;default:''" json:"description"`

	PriceFen int64 `gorm:"column:price_fen;type:bigint;not null" json:"price_fen"`

	Status    int8      `gorm:"type:tinyint;not null;default:2;index:idx_products_status" json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Product) TableName() string {
	return "products"
}
