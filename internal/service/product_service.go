package service

import (
	"context"
	"errors"
	"go-order-management-system/internal/dao"
	"go-order-management-system/internal/model"
	"go-order-management-system/internal/request"
	"strings"

	"gorm.io/gorm"
)

func (p *ProductService) CreateProduct(ctx context.Context, req request.CreateProductRequest) (*model.Product, error) {
	name := strings.TrimSpace(req.Name)
	description := strings.TrimSpace(req.Description)

	if req.PriceFen <= 0 {
		return nil, ErrInvalidProductPrice
	}

	if name == "" {
		return nil, ErrInvalidProductName
	}

	if len(description) > 500 {
		return nil, ErrInvalidProductDescription
	}

	product := &model.Product{
		Name:        name,
		Description: description,
		PriceFen:    req.PriceFen,
		Status:      model.ProductStatusOffSale,
	}

	if err := dao.CreateProduct(ctx, p.db, product); err != nil {
		return nil, err
	}

	return product, nil
}

func (p *ProductService) ListProducts(ctx context.Context) ([]*model.Product, error) {
	return dao.ListProducts(ctx, p.db, model.ProductStatusOffSale)
}

func (p *ProductService) GetProductByID(ctx context.Context, id int64) (*model.Product, error) {

	if product, ok := p.cache.GetProductDetail(ctx, id); ok {
		return product, nil
	}

	product, err := dao.GetProductByID(ctx, p.db, id)

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrProductNotFound
		}
		return nil, err
	}

	p.cache.SetProductDetail(ctx, product)

	return product, nil
}

func (p *ProductService) OnSaleProduct(ctx context.Context, id int64) error {

	product, err := dao.GetProductByID(ctx, p.db, id)
	if err != nil {

		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrProductNotFound
		}
		return err

	}

	if product.Status == model.ProductStatusOnSale {
		return nil
	}

	if err := dao.UpdateProductStatus(ctx, p.db, product.ID, model.ProductStatusOnSale); err != nil {
		return ErrProductOnSaleFailed
	}

	p.cache.DeleteProductDetailCache(ctx, id)

	return nil
}

func (p *ProductService) OffSaleProduct(ctx context.Context, id int64) error {

	product, err := dao.GetProductByID(ctx, p.db, id)

	if err != nil {

		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrProductNotFound
		}
		return err

	}

	if product.Status == model.ProductStatusOffSale {
		return nil
	}

	if err := dao.UpdateProductStatus(ctx, p.db, product.ID, model.ProductStatusOffSale); err != nil {
		return ErrProductOffSaleFailed
	}

	p.cache.DeleteProductDetailCache(ctx, id)

	return nil
}
