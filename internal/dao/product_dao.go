package dao

import (
	"context"
	"go-order-management-system/internal/model"

	"gorm.io/gorm"
)

func CreateProduct(ctx context.Context, db *gorm.DB, product *model.Product) error {
	return db.WithContext(ctx).Create(product).Error
}

func ListProducts(ctx context.Context, db *gorm.DB) ([]*model.Product, error) {
	var products []*model.Product
	return products, db.WithContext(ctx).Model(&model.Product{}).Where("status = ?", model.ProductStatusOffSale).Order("id DESC").Find(&products).Error
}

func GetProductByID(ctx context.Context, db *gorm.DB, id int64) (*model.Product, error) {
	var product model.Product
	return &product, db.WithContext(ctx).Where("id = ?", id).First(&product).Error
}

func UpdateProductStatus(ctx context.Context, db *gorm.DB, id int64, status int8) error {
	return db.WithContext(ctx).Model(&model.Product{}).Where("id = ?", id).Update("status", status).Error
}
