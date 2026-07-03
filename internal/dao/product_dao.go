package dao

import (
	"context"
	"go-order-management-system/internal/model"

	"gorm.io/gorm"
)

func CreateProduct(ctx context.Context, db *gorm.DB, product *model.Product) error {
	return db.WithContext(ctx).Create(product).Error
}

func ListProducts(ctx context.Context, db *gorm.DB, status *int8, page, pageSize int) ([]*model.Product, int64, error) {
	var products []*model.Product
	query := db.WithContext(ctx).Model(&model.Product{})
	if status != nil {
		query = query.Where("status = ?", *status)
	}
	var total int64
	if pageSize > 0 {
		if err := query.Count(&total).Error; err != nil {
			return nil, 0, err
		}
		query = query.Offset((page - 1) * pageSize).Limit(pageSize)
	}
	if err := query.Order("id DESC").Find(&products).Error; err != nil {
		return nil, 0, err
	}
	if pageSize == 0 {
		total = int64(len(products))
	}
	return products, total, nil
}

func GetProductByID(ctx context.Context, db *gorm.DB, id int64) (*model.Product, error) {
	var product model.Product
	return &product, db.WithContext(ctx).Where("id = ?", id).First(&product).Error
}

func UpdateProductStatus(ctx context.Context, db *gorm.DB, id int64, status int8) error {
	return db.WithContext(ctx).Model(&model.Product{}).Where("id = ?", id).Update("status", status).Error
}
