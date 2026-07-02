package service

import (
	"context"
	"go-order-management-system/internal/model"

	"gorm.io/gorm"
)

type ProductService struct {
	db    *gorm.DB
	cache ProductCache
}

func NewProductService(db *gorm.DB, cache ProductCache) *ProductService {
	return &ProductService{
		db:    db,
		cache: cache,
	}
}

type ProductCache interface {
	GetProductDetail(ctx context.Context, productID int64) (*model.Product, bool)
	SetProductDetail(ctx context.Context, product *model.Product)
	DeleteProductDetailCache(ctx context.Context, productID int64)
}
