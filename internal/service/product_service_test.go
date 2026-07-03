package service_test

import (
	"context"
	"errors"
	"go-order-management-system/internal/bizcache"
	"go-order-management-system/internal/model"
	"go-order-management-system/internal/request"
	"go-order-management-system/internal/service"
	"strings"
	"testing"

	"gorm.io/gorm"
)

func newProductService(t *testing.T) (*gorm.DB, *service.ProductService) {
	t.Helper()
	testDB := setupTestDB(t)
	cache := bizcache.NewProductCache(nil)
	productSvc := service.NewProductService(testDB, cache)
	return testDB, productSvc
}

type fakeProductCache struct {
	product          *model.Product
	hit              bool
	setProduct       *model.Product
	deletedProductID int64
}

func (f *fakeProductCache) GetProductDetail(context.Context, int64) (*model.Product, bool) {
	return f.product, f.hit
}

func (f *fakeProductCache) SetProductDetail(_ context.Context, product *model.Product) {
	f.setProduct = product
}

func (f *fakeProductCache) DeleteProductDetailCache(_ context.Context, productID int64) {
	f.deletedProductID = productID
}

func TestCreateProduct_Success(t *testing.T) {
	testDB, productSvc := newProductService(t)

	req := request.CreateProductRequest{
		Name:        "test product",
		Description: "desc",
		PriceFen:    199,
	}
	product, err := productSvc.CreateProduct(context.Background(), req)
	if err != nil {
		t.Fatalf("create product failed: %v", err)
	}

	if product.ID <= 0 {
		t.Fatalf("expected product ID > 0,got %d", product.ID)
	}

	if product.Name != req.Name {
		t.Fatalf("expected name %q,got %q", req.Name, product.Name)
	}

	if product.Description != req.Description {
		t.Fatalf("expected description %q,got %q", req.Description, product.Description)
	}

	if product.PriceFen != req.PriceFen {
		t.Fatalf("expected price %d,got %d", req.PriceFen, product.PriceFen)
	}

	var saved model.Product

	if err := testDB.First(&saved, product.ID).Error; err != nil {
		t.Fatalf("query product record failed: %v", err)
	}

	if saved.Name != req.Name || saved.Description != req.Description || saved.PriceFen != req.PriceFen || saved.Status != model.ProductStatusOffSale {
		t.Fatalf("saved record mismatch, got %+v", saved)
	}

}

func TestCreateProduct_InvalidPrice(t *testing.T) {
	_, productSvc := newProductService(t)

	req := request.CreateProductRequest{
		Name:        "test product",
		Description: "desc",
		PriceFen:    0,
	}

	product, err := productSvc.CreateProduct(context.Background(), req)
	if !errors.Is(err, service.ErrInvalidProductPrice) {
		t.Fatalf("expected ErrInvalidProductPrice, got err=%v", err)
	}
	if product != nil {
		t.Fatalf("expected nil product, got %+v", product)
	}
}

func TestCreateProduct_SuccessTrimmed(t *testing.T) {
	_, productSvc := newProductService(t)

	req := request.CreateProductRequest{
		Name:        "  apple  ",
		Description: "  good  ",
		PriceFen:    199,
	}

	p, err := productSvc.CreateProduct(context.Background(), req)
	if err != nil {
		t.Fatalf("create product failed: %v", err)
	}
	if p.Name != "apple" || p.Description != "good" {
		t.Fatalf("trim failed, got name=%q description=%q", p.Name, p.Description)
	}
	if p.Status != model.ProductStatusOffSale {
		t.Fatalf("unexpected status: %d", p.Status)
	}
}

func TestCreateProduct_EmptyName(t *testing.T) {
	testDB, productSvc := newProductService(t)

	req := request.CreateProductRequest{
		Name:        "",
		Description: "name is empty",
		PriceFen:    199,
	}

	product, err := productSvc.CreateProduct(context.Background(), req)
	if !errors.Is(err, service.ErrInvalidProductName) {
		t.Fatalf("expected ErrInvalidProductName, got err=%v", err)
	}

	if product != nil {
		t.Fatalf("expected product nil, got %+v", product)
	}

	var count int64
	if err := testDB.Model(&model.Product{}).Where("description = ? AND name = ? ", req.Description, req.Name).Count(&count).Error; err != nil {
		t.Fatalf("count products failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 products, got %d", count)
	}
}

func TestCreateProduct_DescriptionTooLong(t *testing.T) {
	testDB, productSvc := newProductService(t)

	req := request.CreateProductRequest{
		Name:        "description-too-long-product",
		Description: strings.Repeat("a", 501),
		PriceFen:    199,
	}

	product, err := productSvc.CreateProduct(context.Background(), req)
	if !errors.Is(err, service.ErrInvalidProductDescription) {
		t.Fatalf("expect desciption less 500 character:,got %v", err)
	}

	if product != nil {
		t.Fatalf("expect product nil,got %+v", product)
	}

	var count int64
	if err := testDB.Model(&model.Product{}).Where("name = ? AND price_fen = ?", req.Name, 199).Count(&count).Error; err != nil {
		t.Fatalf("count products failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 products, got %d", count)
	}
}

func TestCreateProduct_DescriptionExactly500(t *testing.T) {
	testDB, productSvc := newProductService(t)

	req := request.CreateProductRequest{
		Name:        "description exactly 500",
		Description: strings.Repeat("a", 500),
		PriceFen:    199,
	}

	product, err := productSvc.CreateProduct(context.Background(), req)
	if err != nil {
		t.Fatalf("create product failed: %v", err)
	}

	if product == nil {
		t.Fatal("expected product not nil")
		return
	}

	if product.ID <= 0 {
		t.Fatalf("expected product ID > 0, got %d", product.ID)
	}

	if product.Name != req.Name {
		t.Fatalf("expected name %q, got %q", req.Name, product.Name)
	}

	if product.PriceFen != req.PriceFen {
		t.Fatalf("expected price_fen %d, got %d", req.PriceFen, product.PriceFen)
	}

	if product.Description != req.Description {
		t.Fatalf("expected description %q, got %q", req.Description, product.Description)
	}

	if len(product.Description) != 500 {
		t.Fatalf("expected description length 500, got %d", len(product.Description))
	}

	if product.Status != model.ProductStatusOffSale {
		t.Fatalf("expected status off-sale, got %d", product.Status)
	}

	var saved model.Product
	if err := testDB.First(&saved, product.ID).Error; err != nil {
		t.Fatalf("query product failed: %v", err)
	}

	if saved.Name != req.Name || saved.Description != req.Description || saved.PriceFen != req.PriceFen || saved.Status != model.ProductStatusOffSale {
		t.Fatalf("saved record mismatch, got %+v", saved)
	}
}

func TestListProducts_OnlyOffSale(t *testing.T) {
	testDB, productSvc := newProductService(t)

	seedProduct(t, testDB, "off-sale", 100, model.ProductStatusOffSale)
	seedProduct(t, testDB, "on-sale", 100, model.ProductStatusOnSale)

	products, err := productSvc.ListProducts(context.Background())
	if err != nil {
		t.Fatalf("list products failed: %v", err)
	}

	if len(products) != 1 {
		t.Fatalf("expected 1 off-sale product, got %d", len(products))
	}
	if products[0].Status != model.ProductStatusOffSale {
		t.Fatalf("unexpected status: %d", products[0].Status)
	}
}

func TestGetProductByID_NotFound(t *testing.T) {
	_, productSvc := newProductService(t)

	_, err := productSvc.GetProductByID(context.Background(), 99999)
	if !errors.Is(err, service.ErrProductNotFound) {
		t.Fatalf("expected ErrProductNotFound, got %v", err)
	}
}

func TestGetProductByID_CacheHit(t *testing.T) {
	expected := &model.Product{
		ID:       1001,
		Name:     "cached product",
		PriceFen: 100,
		Status:   model.ProductStatusOnSale,
	}
	cache := &fakeProductCache{product: expected, hit: true}
	productSvc := service.NewProductService(nil, cache)

	got, err := productSvc.GetProductByID(context.Background(), expected.ID)
	if err != nil {
		t.Fatalf("get cached product failed: %v", err)
	}
	if got != expected {
		t.Fatalf("expected cached product %+v, got %+v", expected, got)
	}
	if cache.setProduct != nil {
		t.Fatalf("cache hit should not set cache, got %+v", cache.setProduct)
	}
}

func TestGetProductByID_CacheMissSetsCache(t *testing.T) {
	testDB := setupTestDB(t)
	expected := seedProduct(t, testDB, "cache-miss-product", 100, model.ProductStatusOnSale)
	cache := &fakeProductCache{}
	productSvc := service.NewProductService(testDB, cache)

	got, err := productSvc.GetProductByID(context.Background(), expected.ID)
	if err != nil {
		t.Fatalf("get product failed: %v", err)
	}
	if got.ID != expected.ID {
		t.Fatalf("expected product ID %d, got %d", expected.ID, got.ID)
	}
	if cache.setProduct == nil || cache.setProduct.ID != expected.ID {
		t.Fatalf("expected product %d to be cached, got %+v", expected.ID, cache.setProduct)
	}
}

func TestOnSaleProduct_Success(t *testing.T) {
	testDB := setupTestDB(t)
	cache := &fakeProductCache{}
	productSvc := service.NewProductService(testDB, cache)

	p := seedProduct(t, testDB, "p1", 100, model.ProductStatusOffSale)
	if err := productSvc.OnSaleProduct(context.Background(), p.ID); err != nil {
		t.Fatalf("on sale failed: %v", err)
	}

	var got model.Product
	if err := testDB.First(&got, p.ID).Error; err != nil {
		t.Fatalf("query product failed: %v", err)
	}
	if got.Status != model.ProductStatusOnSale {
		t.Fatalf("expected on-sale status, got %d", got.Status)
	}
	if cache.deletedProductID != p.ID {
		t.Fatalf("expected cache for product %d to be deleted, got %d", p.ID, cache.deletedProductID)
	}
}

func TestOffSaleProduct_Success(t *testing.T) {
	testDB := setupTestDB(t)
	cache := &fakeProductCache{}
	productSvc := service.NewProductService(testDB, cache)

	p := seedProduct(t, testDB, "p1", 100, model.ProductStatusOnSale)
	if err := productSvc.OffSaleProduct(context.Background(), p.ID); err != nil {
		t.Fatalf("off sale failed: %v", err)
	}

	var got model.Product
	if err := testDB.First(&got, p.ID).Error; err != nil {
		t.Fatalf("query product failed: %v", err)
	}
	if got.Status != model.ProductStatusOffSale {
		t.Fatalf("expected off-sale status, got %d", got.Status)
	}
	if cache.deletedProductID != p.ID {
		t.Fatalf("expected cache for product %d to be deleted, got %d", p.ID, cache.deletedProductID)
	}
}
