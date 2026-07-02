package service_test

import (
	"context"
	"go-order-management-system/config"
	"go-order-management-system/internal/model"
	"go-order-management-system/internal/request"
	"go-order-management-system/internal/service"
	"go-order-management-system/pkg/database"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"gorm.io/gorm"
)

const testUserID int64 = 1
const otherTestUserID int64 = 2

func newIdempotencyKey() string {
	return uuid.NewString()
}

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	if os.Getenv("RUN_MYSQL_TEST") != "1" {
		t.Skip("skip MySQL integration test; set RUN_MYSQL_TEST=1 to run")
	}

	_ = godotenv.Load("../../.env")

	cfg, err := config.LoadConfig("../../config.yml")
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}

	testDB, err := database.InitTestMySQL(cfg)
	if err != nil {
		t.Fatalf("init test MySQL failed: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, err := testDB.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	err = testDB.AutoMigrate(&model.User{}, &model.Product{}, &model.Inventory{}, &model.StockLog{}, &model.Order{}, &model.OrderItem{}, &model.OrderIdempotencyKey{})

	if err != nil {
		t.Fatalf("migrate test MySQL failed: %v", err)
	}

	cleanTables(t, testDB)
	users := []*model.User{
		{ID: testUserID, Username: "order-test-user", PasswordHash: "not-used", Nickname: "order-test-user", Status: model.UserStatusActive},
		{ID: otherTestUserID, Username: "other-order-test-user", PasswordHash: "not-used", Nickname: "other-order-test-user", Status: model.UserStatusActive},
	}
	if err := testDB.Create(&users).Error; err != nil {
		t.Fatalf("seed test users failed: %v", err)
	}

	return testDB
}

func cleanTables(t *testing.T, db *gorm.DB) {
	t.Helper()

	tables := []string{
		"order_idempotency_keys",
		"stock_logs",
		"order_items",
		"orders",
		"product_inventories",
		"products",
		"users",
	}

	for _, table := range tables {
		if err := db.Exec("DELETE FROM " + table).Error; err != nil {
			t.Fatalf("clean table %s failed: %v", table, err)
		}
	}
}

func seedProduct(t *testing.T, testDB *gorm.DB, name string, priceFen int64, status int8) *model.Product {
	t.Helper()
	p := &model.Product{
		Name:        name,
		Description: name + "-desc",
		PriceFen:    priceFen,
		Status:      status,
	}
	if err := testDB.Create(p).Error; err != nil {
		t.Fatalf("seed product failed: %v", err)
	}
	return p
}

func seedInventory(t *testing.T, testDB *gorm.DB, productID int64, qty int64) *model.Inventory {
	t.Helper()
	inv := &model.Inventory{
		ProductID:     productID,
		StockQuantity: qty,
	}
	if err := testDB.Create(inv).Error; err != nil {
		t.Fatalf("seed inventory failed: %v", err)
	}
	return inv
}

func seedPendingOrder(t *testing.T, testDB *gorm.DB) *model.Order {
	t.Helper()

	db := testDB
	orderSvc := service.NewOrderService(db)
	name := "order pending test"
	priceFen := int64(100)
	qty := int64(50)
	orderQty := int64(25)

	product := &model.Product{
		Name:        name,
		Description: name + "desc",
		Status:      model.ProductStatusOnSale,
		PriceFen:    priceFen,
	}

	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("create product failed: %v", err)
	}

	inventory := &model.Inventory{
		ProductID:     product.ID,
		StockQuantity: qty,
	}

	if err := db.Create(&inventory).Error; err != nil {
		t.Fatalf("create inventory failed: %v", err)
	}

	order, err := orderSvc.CreateOrder(context.Background(), testUserID, request.CreateOrderRequest{
		IdempotencyKey: newIdempotencyKey(),
		Items: []request.CreateOrderItemRequest{
			{ProductID: product.ID,
				Quantity: orderQty},
		},
	})

	if err != nil {
		t.Fatalf("create order failed: %v", err)
	}

	return order
}

func seedPaidOrder(t *testing.T, testDB *gorm.DB) *model.Order {
	t.Helper()

	orderSvc := service.NewOrderService(testDB)
	name := "order paid test"
	priceFen := int64(100)
	qty := int64(50)
	orderQty := int64(25)

	product := seedProduct(t, testDB, name, priceFen, model.ProductStatusOnSale)

	seedInventory(t, testDB, product.ID, qty)

	order, err := orderSvc.CreateOrder(context.Background(), testUserID, request.CreateOrderRequest{
		IdempotencyKey: newIdempotencyKey(),
		Items: []request.CreateOrderItemRequest{
			{ProductID: product.ID, Quantity: orderQty},
		},
	})

	if err != nil {
		t.Fatalf("create order failed: %v", err)
	}

	if err := orderSvc.PayOrder(context.Background(), testUserID, order.ID); err != nil {
		t.Fatalf("pay order failed: %v", err)
	}

	return order
}

func seedFinishedOrder(t *testing.T, testDB *gorm.DB) *model.Order {
	t.Helper()

	orderSvc := service.NewOrderService(testDB)
	order := seedPaidOrder(t, testDB)

	if err := orderSvc.FinishOrder(context.Background(), testUserID, order.ID); err != nil {
		t.Fatalf("finish order failed: %v", err)
	}

	return order
}

type seededOrderContext struct {
	Order    *model.Order
	Product  *model.Product
	InitQty  int64
	OrderQty int64
}

func seedPendingOrderContext(t *testing.T, testDB *gorm.DB) seededOrderContext {
	t.Helper()

	db := testDB
	orderSvc := service.NewOrderService(db)
	name := "order pending test"
	priceFen := int64(100)
	qty := int64(50)
	orderQty := int64(25)

	product := &model.Product{
		Name:        name,
		Description: name + "desc",
		Status:      model.ProductStatusOnSale,
		PriceFen:    priceFen,
	}

	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("create product failed: %v", err)
	}

	inventory := &model.Inventory{
		ProductID:     product.ID,
		StockQuantity: qty,
	}

	if err := db.Create(&inventory).Error; err != nil {
		t.Fatalf("create inventory failed: %v", err)
	}

	order, err := orderSvc.CreateOrder(context.Background(), testUserID, request.CreateOrderRequest{
		IdempotencyKey: newIdempotencyKey(),
		Items: []request.CreateOrderItemRequest{
			{ProductID: product.ID,
				Quantity: orderQty},
		},
	})

	if err != nil {
		t.Fatalf("create order failed: %v", err)
	}

	return seededOrderContext{
		Order:    order,
		Product:  product,
		InitQty:  qty,
		OrderQty: orderQty,
	}
}
