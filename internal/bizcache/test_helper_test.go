package bizcache_test

import (
	"go-order-management-system/config"
	"go-order-management-system/pkg/redis"
	"os"
	"testing"

	goredis "github.com/redis/go-redis/v9"
)

func setupTestRedis(t *testing.T) *goredis.Client {
	t.Helper()

	if os.Getenv("RUN_REDIS_TEST") != "1" {
		t.Skip("skip redis integration test; set RUN_REDIS_TEST=1 to run")
	}

	cfg, err := config.LoadConfig("../../config.yml")
	if err != nil {
		t.Skipf("load redis config failed: %v", err)
	}

	client, err := redis.InitRedis(cfg)
	if err != nil {
		t.Skipf("init redis failed: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Close()
	})

	return client
}
