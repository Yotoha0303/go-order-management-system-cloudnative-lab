package bizcache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go-order-management-system/internal/model"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

type ProductCache struct {
	redisClient *redis.Client
}

func NewProductCache(redisClient *redis.Client) *ProductCache {
	return &ProductCache{
		redisClient: redisClient,
	}
}

const ProductDetailCacheTTL = 10 * time.Minute

func ProductDetailCacheKey(productID int64) string {
	return fmt.Sprintf("product:detail:%d", productID)
}

func (p *ProductCache) GetProductDetail(ctx context.Context, productID int64) (*model.Product, bool) {
	if p.redisClient == nil {
		return nil, false
	}

	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	val, err := p.redisClient.Get(ctx, ProductDetailCacheKey(productID)).Result()
	if err != nil {

		if errors.Is(err, redis.Nil) {
			return nil, false
		}

		log.Printf("get product cache failed: product_id=%d err=%v", productID, err)
		return nil, false
	}

	var product model.Product
	if err := json.Unmarshal([]byte(val), &product); err != nil {
		return nil, false
	}

	return &product, true
}

func (p *ProductCache) SetProductDetail(ctx context.Context, product *model.Product) {
	if p.redisClient == nil || product == nil {
		return
	}

	data, err := json.Marshal(product)
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	_ = p.redisClient.Set(ctx, ProductDetailCacheKey(product.ID), data, ProductDetailCacheTTL).Err()
}

func (p *ProductCache) DeleteProductDetailCache(ctx context.Context, productID int64) {
	if p.redisClient == nil {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	_ = p.redisClient.Del(ctx, ProductDetailCacheKey(productID)).Err()
}
