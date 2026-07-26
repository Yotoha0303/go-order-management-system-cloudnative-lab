package ordertimeout

import (
	"context"
	"database/sql"
	"log/slog"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"go-order-management-system-cloudnative-lab/config"
	"go-order-management-system-cloudnative-lab/internal/model"
	"go-order-management-system-cloudnative-lab/internal/request"
	"go-order-management-system-cloudnative-lab/internal/service"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	amqp "github.com/rabbitmq/amqp091-go"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestOrderTimeoutEndToEnd(t *testing.T) {
	if os.Getenv("RUN_RABBITMQ_TEST") != "1" || os.Getenv("RUN_MYSQL_TEST") != "1" {
		t.Skip("skip order timeout end-to-end test; enable RabbitMQ and MySQL integration tests")
	}
	_ = godotenv.Load("../../.env")
	db := setupIsolatedOrderTimeoutDatabase(t)
	rabbitURL := os.Getenv("RABBITMQ_URL")
	if rabbitURL == "" {
		rabbitURL = "amqp://order_app:order_dev_password@127.0.0.1:5672/"
	}

	user := &model.User{
		Username:     "timeout-" + uuid.NewString(),
		PasswordHash: "not-used",
		Nickname:     "timeout-test",
		Status:       model.UserStatusActive,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create test user: %v", err)
	}
	product := &model.Product{
		Name:        "timeout-product-" + uuid.NewString(),
		Description: "order timeout end-to-end test",
		PriceFen:    100,
		Status:      model.ProductStatusOnSale,
	}
	if err := db.Create(product).Error; err != nil {
		t.Fatalf("create test product: %v", err)
	}
	if err := db.Create(&model.Inventory{ProductID: product.ID, StockQuantity: 10}).Error; err != nil {
		t.Fatalf("create test inventory: %v", err)
	}

	const orderDelay = 300 * time.Millisecond
	orderService := service.NewOrderServiceWithTimeout(db, orderDelay)
	createdRequestAt := time.Now()
	order, err := orderService.CreateOrder(context.Background(), user.ID, request.CreateOrderRequest{
		IdempotencyKey: uuid.NewString(),
		Items:          []request.CreateOrderItemRequest{{ProductID: product.ID, Quantity: 3}},
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	var scheduled model.OrderTimeoutOutbox
	if err := db.Where("order_id = ?", order.ID).First(&scheduled).Error; err != nil {
		t.Fatalf("query scheduled timeout: %v", err)
	}
	t.Logf(
		"created_request_at=%s order_created_at=%s timeout_at=%s remaining=%s",
		createdRequestAt.Format(time.RFC3339Nano),
		order.CreatedAt.Format(time.RFC3339Nano),
		scheduled.TimeoutAt.Format(time.RFC3339Nano),
		time.Until(scheduled.TimeoutAt),
	)

	worker, err := NewWorker(Config{
		URL:                rabbitURL,
		ConnectTimeout:     2 * time.Second,
		ReconnectDelay:     100 * time.Millisecond,
		OutboxPollInterval: 25 * time.Millisecond,
		OutboxRetryDelay:   50 * time.Millisecond,
		PublishBatchSize:   10,
		ConsumerPrefetch:   1,
	}, db, orderService, slog.New(slog.NewTextHandler(testLogWriter{t}, nil)))
	if err != nil {
		t.Fatalf("NewWorker: %v", err)
	}
	worker.topology = uniqueTestTopology()
	t.Cleanup(func() { deleteTopologyByURL(t, rabbitURL, worker.topology) })

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- worker.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("worker shutdown: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("worker did not stop")
		}
	})

	deadline := time.Now().Add(10 * time.Second)
	for {
		var stored model.Order
		if err := db.First(&stored, order.ID).Error; err != nil {
			t.Fatalf("query order: %v", err)
		}
		if stored.Status == model.OrderStatusCancelled {
			if elapsed := time.Since(createdRequestAt); elapsed < 250*time.Millisecond {
				t.Fatalf("order cancelled before configured delay: %s", elapsed)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("order status=%d, timeout waiting for automatic cancellation", stored.Status)
		}
		time.Sleep(25 * time.Millisecond)
	}

	var inventory model.Inventory
	if err := db.Where("product_id = ?", product.ID).First(&inventory).Error; err != nil {
		t.Fatalf("query inventory: %v", err)
	}
	if inventory.StockQuantity != 10 {
		t.Fatalf("inventory=%d want=10", inventory.StockQuantity)
	}
	var outbox model.OrderTimeoutOutbox
	if err := db.Where("order_id = ?", order.ID).First(&outbox).Error; err != nil {
		t.Fatalf("query outbox: %v", err)
	}
	if outbox.PublishedAt == nil {
		t.Fatal("outbox was not marked published after RabbitMQ confirmation")
	}
	var rollbackCount int64
	if err := db.Model(&model.StockLog{}).
		Where("biz_id = ? AND biz_type = ?", order.ID, model.StockBizOrderRollback).
		Count(&rollbackCount).Error; err != nil {
		t.Fatalf("count rollback logs: %v", err)
	}
	if rollbackCount != 1 {
		t.Fatalf("rollback logs=%d want=1", rollbackCount)
	}
}

func setupIsolatedOrderTimeoutDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	cfg, err := config.LoadConfig("../../config.yml")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	databaseName := "go_order_timeout_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	driverConfig := mysqldriver.Config{
		User:      cfg.MySQL.User,
		Passwd:    os.Getenv("MYSQL_TEST_PASSWORD"),
		Net:       "tcp",
		Addr:      net.JoinHostPort(cfg.MySQL.Host, cfg.MySQL.Port),
		ParseTime: true,
		Loc:       time.Local,
	}
	admin, err := sql.Open("mysql", driverConfig.FormatDSN())
	if err != nil {
		t.Fatalf("open MySQL admin connection: %v", err)
	}
	if err := admin.Ping(); err != nil {
		_ = admin.Close()
		t.Fatalf("ping MySQL: %v", err)
	}
	if _, err := admin.Exec("CREATE DATABASE `" + databaseName + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci"); err != nil {
		_ = admin.Close()
		t.Fatalf("create isolated database: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec("DROP DATABASE IF EXISTS `" + databaseName + "`")
		_ = admin.Close()
	})

	driverConfig.DBName = databaseName
	db, err := gorm.Open(gormmysql.Open(driverConfig.FormatDSN()), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("open isolated database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get isolated database handle: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(
		&model.User{},
		&model.Product{},
		&model.Inventory{},
		&model.StockLog{},
		&model.Order{},
		&model.OrderItem{},
		&model.OrderIdempotencyKey{},
		&model.OrderTimeoutOutbox{},
	); err != nil {
		t.Fatalf("migrate isolated database: %v", err)
	}
	return db
}

func uniqueTestTopology() topology {
	suffix := uuid.NewString()
	return topology{
		exchange:         "test.order.timeout." + suffix,
		delayQueue:       "test.order.timeout.delay." + suffix,
		cancelQueue:      "test.order.timeout.cancel." + suffix,
		failedQueue:      "test.order.timeout.failed." + suffix,
		delayRoutingKey:  "delay",
		cancelRoutingKey: "cancel",
		failedRoutingKey: "failed",
	}
}

func deleteTopologyByURL(t *testing.T, url string, topology topology) {
	t.Helper()
	connection, err := amqp.DialConfig(url, amqp.Config{Dial: amqp.DefaultDial(5 * time.Second)})
	if err != nil {
		t.Logf("connect for RabbitMQ cleanup: %v", err)
		return
	}
	defer connection.Close()
	deleteTestTopology(t, connection, topology)
}

type testLogWriter struct{ t *testing.T }

func (w testLogWriter) Write(data []byte) (int, error) {
	w.t.Log(strings.TrimSpace(string(data)))
	return len(data), nil
}
