package redis

import (
	"context"
	"fmt"
	"go-order-management-system/config"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	ErrRedisPasswordNotFound = fmt.Errorf("redis password missing")
	ErrRedisAddrNotFound     = fmt.Errorf("redis addr missing")
)

func InitRedis(cfg *config.Config) (*redis.Client, error) {
	return buildRedisClient(cfg)
}

func buildRedisClient(cfg *config.Config) (*redis.Client, error) {

	password := os.Getenv("REDIS_PASSWORD")

	if cfg.Redis.Addr == "" {
		return nil, ErrRedisAddrNotFound
	}

	client := openRedisClient(cfg, password)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return client, nil
}

func openRedisClient(cfg *config.Config, password string) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: password,
		DB:       cfg.Redis.DB,
	})
}
