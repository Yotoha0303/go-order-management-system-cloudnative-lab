package bizcache_test

import (
	"context"
	"go-order-management-system/internal/bizcache"
	"go-order-management-system/internal/model"
	"testing"
)

func newTestProductCache() *bizcache.ProductCache {
	return bizcache.NewProductCache(nil)
}

func TestProductDetailCacheKey(t *testing.T) {

	got := bizcache.ProductDetailCacheKey(1001)
	want := "product:detail:1001"
	if got != want {
		t.Fatalf("productDetailCacheKey(1001) = %q, want %q", got, want)
	}

}

func TestProductDetailCache_NoRedis(t *testing.T) {

	cache := newTestProductCache()

	var noExistProductID = int64(1002)
	ctx := context.Background()

	product, ok := cache.GetProductDetail(ctx, noExistProductID)
	if ok {
		t.Fatalf("expected cache miss when redis is nil, got hit: %+v", product)
	}

	cache.SetProductDetail(ctx, &model.Product{ID: noExistProductID, Name: "test no redis"})
	cache.DeleteProductDetailCache(ctx, noExistProductID)
}

func TestProductDetailCache_SetGet_WithRedis(t *testing.T) {
	client := setupTestRedis(t)
	cache := bizcache.NewProductCache(client)

	ctx := context.Background()
	product := &model.Product{
		ID:          1004,
		Name:        "product detail on redis to set and get",
		Description: "desc",
		PriceFen:    10,
		Status:      model.ProductStatusOnSale,
	}

	cache.SetProductDetail(ctx, product)
	got, ok := cache.GetProductDetail(ctx, product.ID)
	if !ok {
		t.Fatalf("expected product detail cache exist")
	}

	if got.ID != product.ID || got.Name != product.Name || got.Description != product.Description || got.PriceFen != product.PriceFen || got.Status != product.Status {
		t.Fatalf("product mismatch,got %+v,want %+v", got, product)
	}

	defer cache.DeleteProductDetailCache(ctx, product.ID)
}

func TestProductDetailCache_DeleteMiss_WithRedis(t *testing.T) {
	client := setupTestRedis(t)
	ctx := context.Background()

	product := &model.Product{
		ID:          1005,
		Name:        "product detail get miss",
		Description: "desc",
		PriceFen:    10,
		Status:      model.ProductStatusOnSale,
	}
	cache := bizcache.NewProductCache(client)

	cache.SetProductDetail(ctx, product)
	p, ok := cache.GetProductDetail(ctx, product.ID)

	if !ok {
		t.Fatalf("expected product detail cache found")
	}

	if p.ID != product.ID || p.Name != product.Name || p.Description != product.Description || p.PriceFen != product.PriceFen || p.Status != product.Status {
		t.Fatalf("product mismatch, got %+v, want %+v", p, product)
	}

	cache.DeleteProductDetailCache(ctx, product.ID)
	p, ok = cache.GetProductDetail(ctx, product.ID)

	if ok {
		t.Fatalf("expected product detail cache not found")
	}

	if p != nil {
		t.Fatalf("expected product detail cache not found,got %v", p)
	}

	defer cache.DeleteProductDetailCache(context.Background(), product.ID)
}

func TestProductDetailCache_TTL_WithRedis(t *testing.T) {
	client := setupTestRedis(t)

	ctx := context.Background()
	product := &model.Product{
		ID:          1006,
		Name:        "product detail ttl",
		Description: "desc",
		PriceFen:    10,
		Status:      model.ProductStatusOnSale,
	}

	cache := bizcache.NewProductCache(client)

	cache.SetProductDetail(ctx, product)

	ttl, err := client.TTL(ctx, bizcache.ProductDetailCacheKey(product.ID)).Result()
	if err != nil {
		t.Fatalf("query ttl failed: %v", err)
	}
	if ttl <= 0 {
		t.Fatalf("expected ttl > 0, got %v", ttl)
	}
	if ttl > bizcache.ProductDetailCacheTTL {
		t.Fatalf("expected ttl <= %v, got %v", bizcache.ProductDetailCacheTTL, ttl)
	}

	defer cache.DeleteProductDetailCache(context.Background(), product.ID)
}
