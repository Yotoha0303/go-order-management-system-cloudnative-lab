package dao

import (
	"context"
	"go-order-management-system/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func InitInventory(ctx context.Context, db *gorm.DB, inventory *model.Inventory) error {
	return db.WithContext(ctx).Create(inventory).Error
}

func GetInventoryByProductID(ctx context.Context, db *gorm.DB, productID int64) (*model.Inventory, error) {
	var inventory model.Inventory
	return &inventory, db.WithContext(ctx).Where("product_id = ?", productID).First(&inventory).Error
}

func GetInventoryByProductIDForUpdate(ctx context.Context, db *gorm.DB, productID int64) (*model.Inventory, error) {
	var inventory model.Inventory
	return &inventory, db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("product_id = ?", productID).First(&inventory).Error
}

func UpdateInventoryStockQuantity(ctx context.Context, db *gorm.DB, productID int64, stockQuantity int64) error {

	result := db.WithContext(ctx).Model(&model.Inventory{}).Where("product_id = ?", productID).Update("stock_quantity", stockQuantity)

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}

	return nil
}

func DeductInventory(ctx context.Context, db *gorm.DB, productID int64, quantity int64) (int64, error) {

	result := db.WithContext(ctx).Model(&model.Inventory{}).Where("product_id = ? AND stock_quantity >= ?", productID, quantity).
		UpdateColumn("stock_quantity", gorm.Expr("stock_quantity - ?", quantity))

	return result.RowsAffected, result.Error
}
