package service_test

import (
	"context"
	"errors"
	"go-order-management-system/internal/model"
	"go-order-management-system/internal/request"
	"go-order-management-system/internal/service"
	"sync"
	"testing"

	"gorm.io/gorm"
)

func newOrderService(t *testing.T) (*gorm.DB, *service.OrderService) {
	t.Helper()
	testDB := setupTestDB(t)
	return testDB, service.NewOrderService(testDB)
}

func TestCreateOrder_InsufficientStock(t *testing.T) {
	testDB, orderSvc := newOrderService(t)
	p := seedProduct(t, testDB, "p1", 100, model.ProductStatusOnSale)
	seedInventory(t, testDB, p.ID, 1)

	_, err := orderSvc.CreateOrder(context.Background(), testUserID, request.CreateOrderRequest{
		IdempotencyKey: newIdempotencyKey(),
		Items: []request.CreateOrderItemRequest{
			{ProductID: p.ID, Quantity: 2},
		},
	})

	if !errors.Is(err, service.ErrInsufficientStock) {
		t.Fatalf("expected ErrInsufficientStock, got %v", err)
	}
}

func TestCreateOrder_ProductNotFound(t *testing.T) {
	_, orderSvc := newOrderService(t)

	_, err := orderSvc.CreateOrder(context.Background(), testUserID, request.CreateOrderRequest{
		IdempotencyKey: newIdempotencyKey(),
		Items: []request.CreateOrderItemRequest{
			{ProductID: 999999, Quantity: 1},
		},
	})

	if !errors.Is(err, service.ErrProductNotFound) {
		t.Fatalf("expected ErrProductNotFound, got %v", err)
	}
}

func TestCreateOrder_ProductOffSale(t *testing.T) {
	testDB, orderSvc := newOrderService(t)
	product := seedProduct(t, testDB, "off-sale-product", 100, model.ProductStatusOffSale)
	seedInventory(t, testDB, product.ID, 10)

	_, err := orderSvc.CreateOrder(context.Background(), testUserID, request.CreateOrderRequest{
		IdempotencyKey: newIdempotencyKey(),
		Items: []request.CreateOrderItemRequest{
			{ProductID: product.ID, Quantity: 1},
		},
	})

	if !errors.Is(err, service.ErrProductOffSale) {
		t.Fatalf("expected ErrProductOffSale, got %v", err)
	}
}

func TestCreateOrder_InventoryNotFound(t *testing.T) {
	testDB, orderSvc := newOrderService(t)
	product := seedProduct(t, testDB, "no-inventory-product", 100, model.ProductStatusOnSale)

	_, err := orderSvc.CreateOrder(context.Background(), testUserID, request.CreateOrderRequest{
		IdempotencyKey: newIdempotencyKey(),
		Items: []request.CreateOrderItemRequest{
			{ProductID: product.ID, Quantity: 1},
		},
	})

	if !errors.Is(err, service.ErrInventoryNotFound) {
		t.Fatalf("expected ErrInventoryNotFound, got %v", err)
	}
}

func TestCreateOrder_MultipleItemsSecondInsufficient_Rollback(t *testing.T) {
	testDB, orderSvc := newOrderService(t)
	productA := seedProduct(t, testDB, "rollback-product-a", 100, model.ProductStatusOnSale)
	productB := seedProduct(t, testDB, "rollback-product-b", 200, model.ProductStatusOnSale)
	seedInventory(t, testDB, productA.ID, 10)
	seedInventory(t, testDB, productB.ID, 1)

	_, err := orderSvc.CreateOrder(context.Background(), testUserID, request.CreateOrderRequest{
		IdempotencyKey: newIdempotencyKey(),
		Items: []request.CreateOrderItemRequest{
			{ProductID: productA.ID, Quantity: 3},
			{ProductID: productB.ID, Quantity: 2},
		},
	})

	if !errors.Is(err, service.ErrInsufficientStock) {
		t.Fatalf("expected ErrInsufficientStock, got %v", err)
	}

	var invA model.Inventory
	if err := testDB.Where("product_id = ?", productA.ID).First(&invA).Error; err != nil {
		t.Fatalf("query product A inventory failed: %v", err)
	}
	if invA.StockQuantity != 10 {
		t.Fatalf("expected product A inventory rolled back to 10, got %d", invA.StockQuantity)
	}

	var invB model.Inventory
	if err := testDB.Where("product_id = ?", productB.ID).First(&invB).Error; err != nil {
		t.Fatalf("query product B inventory failed: %v", err)
	}
	if invB.StockQuantity != 1 {
		t.Fatalf("expected product B inventory unchanged as 1, got %d", invB.StockQuantity)
	}

	var orderCount int64
	if err := testDB.Model(&model.Order{}).Count(&orderCount).Error; err != nil {
		t.Fatalf("count orders failed: %v", err)
	}
	if orderCount != 0 {
		t.Fatalf("expected no order committed after rollback, got %d", orderCount)
	}

	var orderItemCount int64
	if err := testDB.Model(&model.OrderItem{}).Count(&orderItemCount).Error; err != nil {
		t.Fatalf("count order items failed: %v", err)
	}
	if orderItemCount != 0 {
		t.Fatalf("expected no order item committed after rollback, got %d", orderItemCount)
	}

	var stockLogCount int64
	if err := testDB.Model(&model.StockLog{}).Count(&stockLogCount).Error; err != nil {
		t.Fatalf("count stock logs failed: %v", err)
	}
	if stockLogCount != 0 {
		t.Fatalf("expected no stock log committed after rollback, got %d", stockLogCount)
	}
}

func TestCreateOrder_Success(t *testing.T) {
	testDB, orderSvc := newOrderService(t)
	p := seedProduct(t, testDB, "p1", 100, model.ProductStatusOnSale)
	seedInventory(t, testDB, p.ID, 10)

	order, err := orderSvc.CreateOrder(context.Background(), testUserID, request.CreateOrderRequest{
		IdempotencyKey: newIdempotencyKey(),
		Items: []request.CreateOrderItemRequest{
			{ProductID: p.ID, Quantity: 3},
		},
	})

	if err != nil {
		t.Fatalf("create order failed: %v", err)
	}
	if order.TotalAmountFen != 300 {
		t.Fatalf("expected total_amount_fen=300, got %d", order.TotalAmountFen)
	}
	if order.Status != model.OrderStatusPending {
		t.Fatalf("unexpected order status: %d", order.Status)
	}

	var inv model.Inventory
	if err := testDB.Where("product_id = ?", p.ID).First(&inv).Error; err != nil {
		t.Fatalf("query inventory failed: %v", err)
	}
	if inv.StockQuantity != 7 {
		t.Fatalf("expected stock=7, got %d", inv.StockQuantity)
	}

	var stockLog model.StockLog
	if err := testDB.Where("product_id = ? AND biz_id = ?", p.ID, order.ID).First(&stockLog).Error; err != nil {
		t.Fatalf("query stock log failed:%v", err)
	}

	if stockLog.ChangeQuantity != -3 {
		t.Fatalf("expected change_quantity = -3,got %d", stockLog.ChangeQuantity)
	}

	if stockLog.AfterQuantity != 7 {
		t.Fatalf("expected after quantity = 7,got %d", stockLog.AfterQuantity)
	}

	beforeQuantity := stockLog.AfterQuantity + (-stockLog.ChangeQuantity)
	if beforeQuantity != 10 {
		t.Fatalf("expected before quantity = 10, got %d", beforeQuantity)
	}
}

func TestOrdersAreIsolatedByUserAndIdempotencyKey(t *testing.T) {
	testDB, orderSvc := newOrderService(t)
	product := seedProduct(t, testDB, "isolated-order-product", 100, model.ProductStatusOnSale)
	seedInventory(t, testDB, product.ID, 10)
	key := newIdempotencyKey()
	req := request.CreateOrderRequest{
		IdempotencyKey: key,
		Items:          []request.CreateOrderItemRequest{{ProductID: product.ID, Quantity: 1}},
	}

	first, err := orderSvc.CreateOrder(context.Background(), testUserID, req)
	if err != nil {
		t.Fatalf("create first user order: %v", err)
	}
	second, err := orderSvc.CreateOrder(context.Background(), otherTestUserID, req)
	if err != nil {
		t.Fatalf("same idempotency key must be scoped per user: %v", err)
	}
	if first.ID == second.ID || first.UserID != testUserID || second.UserID != otherTestUserID {
		t.Fatalf("unexpected user orders: first=%+v second=%+v", first, second)
	}

	if _, _, err := orderSvc.GetOrderByID(context.Background(), otherTestUserID, first.ID); !errors.Is(err, service.ErrOrderNotFound) {
		t.Fatalf("other user must not read order, got %v", err)
	}
	orders, err := orderSvc.ListOrders(context.Background(), testUserID)
	if err != nil {
		t.Fatalf("list first user orders: %v", err)
	}
	if len(orders) != 1 || orders[0].ID != first.ID {
		t.Fatalf("expected only first user's order, got %+v", orders)
	}
}

func TestPayOrder_FromPendingToPaid_Success(t *testing.T) {
	testDB, orderSvc := newOrderService(t)

	var initQuantity int64 = 10
	var PriceFen int64 = 100
	var orderedQuantity int64 = 1

	p := seedProduct(t, testDB, "pay-order-product", PriceFen, model.ProductStatusOnSale)
	seedInventory(t, testDB, p.ID, initQuantity)

	order, err := orderSvc.CreateOrder(context.Background(), testUserID, request.CreateOrderRequest{
		IdempotencyKey: newIdempotencyKey(),
		Items: []request.CreateOrderItemRequest{
			{
				ProductID: p.ID,
				Quantity:  orderedQuantity,
			},
		},
	})

	if err != nil {
		t.Fatalf("create order failed: %v", err)
	}

	if order.Status != model.OrderStatusPending {
		t.Fatalf("expected order status pending,got %d", order.Status)
	}
	var invBeforePay model.Inventory
	if err := testDB.Where("product_id = ?", p.ID).First(&invBeforePay).Error; err != nil {
		t.Fatalf("query inventory before pay failed: %v", err)
	}

	expectedStockAfterCreate := initQuantity - orderedQuantity
	if invBeforePay.StockQuantity != expectedStockAfterCreate {
		t.Fatalf("expected stock quantity %d after create order, got %d", expectedStockAfterCreate, invBeforePay.StockQuantity)
	}

	var stockLogCountBeforePay int64
	if err := testDB.Model(&model.StockLog{}).Where("product_id = ? AND biz_id = ? ", p.ID, order.ID).Count(&stockLogCountBeforePay).Error; err != nil {
		t.Fatalf("count stock logs before pay failed: %v", err)
	}

	if stockLogCountBeforePay != 1 {
		t.Fatalf("expected 1 stock log after create order,got %d", stockLogCountBeforePay)
	}

	if err := orderSvc.PayOrder(context.Background(), testUserID, order.ID); err != nil {
		t.Fatalf("pay order failed:%v", err)
	}

	var paidOrder model.Order
	if err := testDB.First(&paidOrder, order.ID).Error; err != nil {
		t.Fatalf("query paid order failed:%v", err)
	}

	if paidOrder.Status != model.OrderStatusPaid {
		t.Fatalf("expected order status paid,got %d", paidOrder.Status)
	}

	if paidOrder.PaidAt == nil {
		t.Fatalf("expected paid_at not nil")
	}

	var invAfterPay model.Inventory
	if err := testDB.Where("product_id = ?", p.ID).First(&invAfterPay).Error; err != nil {
		t.Fatalf("query inventory after pay failed:%v", err)
	}

	if invAfterPay.StockQuantity != expectedStockAfterCreate {
		t.Fatalf("expected stock unchanged after pay:%d,got %d", expectedStockAfterCreate, invAfterPay.StockQuantity)
	}

	var stockLogCountAfterPay int64
	if err := testDB.Model(&model.StockLog{}).Where("product_id = ? AND biz_id = ?", p.ID, order.ID).Count(&stockLogCountAfterPay).Error; err != nil {
		t.Fatalf("count stock logs after pay failed: %v", err)
	}

	if stockLogCountAfterPay != stockLogCountBeforePay {
		t.Fatalf("expected stock log count unchange after pay,before=%d after=%d", stockLogCountBeforePay, stockLogCountAfterPay)
	}

}

func TestPayAndFinishOrder_Success(t *testing.T) {
	testDB, orderSvc := newOrderService(t)
	p := seedProduct(t, testDB, "p1", 100, model.ProductStatusOnSale)
	seedInventory(t, testDB, p.ID, 10)
	order, err := orderSvc.CreateOrder(context.Background(), testUserID, request.CreateOrderRequest{
		IdempotencyKey: newIdempotencyKey(),
		Items: []request.CreateOrderItemRequest{
			{ProductID: p.ID, Quantity: 1},
		},
	})

	if err != nil {
		t.Fatalf("create order failed: %v", err)
	}

	if err := orderSvc.PayOrder(context.Background(), testUserID, order.ID); err != nil {
		t.Fatalf("pay order failed: %v", err)
	}
	if err := orderSvc.FinishOrder(context.Background(), testUserID, order.ID); err != nil {
		t.Fatalf("finish order failed: %v", err)
	}

	var got model.Order
	if err := testDB.First(&got, order.ID).Error; err != nil {
		t.Fatalf("query order failed: %v", err)
	}
	if got.Status != model.OrderStatusFinished {
		t.Fatalf("expected finished status, got %d", got.Status)
	}
}

func TestPayOrder_AlreadyPaid_ReturnsError(t *testing.T) {
	testDB, orderSvc := newOrderService(t)

	order := seedPaidOrder(t, testDB)

	err := orderSvc.PayOrder(context.Background(), testUserID, order.ID)
	if !errors.Is(err, service.ErrOrderAlreadyPaid) {
		t.Fatalf("expected ErrOrderAlreadyPaid,got %v", err)
	}

	var got model.Order
	if err := testDB.First(&got, order.ID).Error; err != nil {
		t.Fatalf("query order failed: %v", err)
	}

	if got.Status != model.OrderStatusPaid {
		t.Fatalf("expected order status still paid,got %d", got.Status)
	}
}

func TestFinishOrder_PendingOrder_ReturnsNotPaidError(t *testing.T) {
	testDB, orderSvc := newOrderService(t)

	order := seedPendingOrder(t, testDB)

	err := orderSvc.FinishOrder(context.Background(), testUserID, order.ID)
	if !errors.Is(err, service.ErrOrderNotPaid) {
		t.Fatalf("expected order unpaid is not finished,got %v", err)
	}

	var got model.Order
	if err := testDB.First(&got, order.ID).Error; err != nil {
		t.Fatalf("query order failed: %v", err)
	}

	if got.Status != model.OrderStatusPending {
		t.Fatalf("expected order status unpaid,got %d", got.Status)
	}

	if got.CompletedAt != nil {
		t.Fatalf("expected completed_at nil,got %v", got.CompletedAt)
	}
}

func TestCancelOrder_Success(t *testing.T) {
	testDB, orderSvc := newOrderService(t)

	db := testDB
	ctx := seedPendingOrderContext(t, testDB)

	if err := orderSvc.CancelOrder(context.Background(), testUserID, ctx.Order.ID); err != nil {
		t.Fatalf("expected order cancel success,got %v", err)
	}

	var got model.Order
	if err := db.First(&got, ctx.Order.ID).Error; err != nil {
		t.Fatalf("query order failed: %v", err)
	}

	if got.Status != model.OrderStatusCancelled {
		t.Fatalf("expected order status already cancel,got %d", got.Status)
	}

	if got.CancelledAt == nil {
		t.Fatalf("expected order cancelled_at not null,got %v", got.CancelledAt)
	}

	var inv model.Inventory
	if err := db.Where("product_id = ?", ctx.Product.ID).First(&inv).Error; err != nil {
		t.Fatalf("query inventory failed: %v", err)
	}

	if inv.StockQuantity != ctx.InitQty {
		t.Fatalf("expected product inventory already rollback,got %d", inv.StockQuantity)
	}

	var rollbackLog model.StockLog
	if err := db.Where("product_id = ? AND biz_id = ? AND biz_type = ?", ctx.Product.ID, ctx.Order.ID, model.StockBizOrderRollback).Order("created_at DESC").First(&rollbackLog).Error; err != nil {
		t.Fatalf("query stock log failed: %v", err)
	}

	if rollbackLog.ChangeQuantity != ctx.OrderQty {
		t.Fatalf("expected rollback change_quantity=%d,got %d", ctx.OrderQty, rollbackLog.ChangeQuantity)
	}

	if rollbackLog.AfterQuantity != ctx.InitQty {
		t.Fatalf("expected stock log after_quantity=%d,got %d", ctx.InitQty, rollbackLog.AfterQuantity)
	}

}

func TestPaidOrder_UnableCancel_ReturnsError(t *testing.T) {
	testDB, orderSvc := newOrderService(t)

	order := seedPaidOrder(t, testDB)
	db := testDB

	err := orderSvc.CancelOrder(context.Background(), testUserID, order.ID)
	if !errors.Is(err, service.ErrOrderAlreadyPaid) {
		t.Fatalf("expected order cancel failed,got %v", err)
	}

	var got model.Order
	if err := db.First(&got, order.ID).Error; err != nil {
		t.Fatalf("query order failed: %v", err)
	}

	if got.Status == model.OrderStatusCancelled {
		t.Fatalf("expected order status is %d,got %d", model.OrderStatusPaid, got.Status)
	}

	if got.CancelledAt != nil {
		t.Fatalf("expected order cancel failed,got %v", got.CancelledAt)
	}
}

func TestFinishedOrder_UnableCancel_ReturnsError(t *testing.T) {
	testDB, orderSvc := newOrderService(t)

	db := testDB

	order := seedFinishedOrder(t, testDB)

	err := orderSvc.CancelOrder(context.Background(), testUserID, order.ID)
	if !errors.Is(err, service.ErrOrderAlreadyFinished) {
		t.Fatalf("expected order cancel failed,got %d", err)
	}

	var got model.Order
	if err := db.First(&got, order.ID).Error; err != nil {
		t.Fatalf("query order failed: %v", err)
	}

	if got.Status == model.OrderStatusCancelled {
		t.Fatalf("expected order status cancelled,got %d", got.Status)
	}

	if got.CancelledAt != nil {
		t.Fatalf("expected order cancel failed,got %v", got.CancelledAt)
	}
}

func TestCancelOrder_CancelledOrder_Idempotent(t *testing.T) {
	testDB, orderSvc := newOrderService(t)

	ctx := seedPendingOrderContext(t, testDB)
	db := testDB

	if err := orderSvc.CancelOrder(context.Background(), testUserID, ctx.Order.ID); err != nil {
		t.Fatalf("order cancenl failed: %v", err)
	}

	var invAfterFirstCancel model.Inventory
	if err := db.Where("product_id = ?", ctx.Product.ID).First(&invAfterFirstCancel).Error; err != nil {
		t.Fatalf("query inventory after first cancel failed: %v", err)
	}

	var rollbackLogCountAfterFirstCancel int64
	if err := db.Model(&model.StockLog{}).Where("product_id = ? AND biz_id = ? AND biz_type = ?", ctx.Product.ID, ctx.Order.ID, model.StockBizOrderRollback).Count(&rollbackLogCountAfterFirstCancel).Error; err != nil {
		t.Fatalf("count rollback stock logs after first cancel failed: %v", err)
	}

	var orderAfterFirstCancel model.Order
	if err := db.First(&orderAfterFirstCancel, ctx.Order.ID).Error; err != nil {
		t.Fatalf("query order after first cancel failed: %v", err)
	}
	if orderAfterFirstCancel.Status != model.OrderStatusCancelled {
		t.Fatalf("expected order status cancelled after first cancel, got %d", orderAfterFirstCancel.Status)
	}
	if orderAfterFirstCancel.CancelledAt == nil {
		t.Fatalf("expected cancelled_at not nil after first cancel")
	}

	if err := orderSvc.CancelOrder(context.Background(), testUserID, ctx.Order.ID); err != nil {
		t.Fatalf("order cancenl failed: %v", err)
	}

	var invAfterSecondCancel model.Inventory
	if err := db.Where("product_id = ?", ctx.Product.ID).First(&invAfterSecondCancel).Error; err != nil {
		t.Fatalf("query inventory after second cancel failed: %v", err)
	}

	if invAfterSecondCancel.StockQuantity != invAfterFirstCancel.StockQuantity {
		t.Fatalf("expected inventory unchanged on second cancel, before=%d after=%d", invAfterFirstCancel.StockQuantity, invAfterSecondCancel.StockQuantity)
	}

	var rollbackLogCountAfterSecondCancel int64
	if err := db.Model(&model.StockLog{}).Where("product_id = ? AND biz_id = ? AND biz_type = ?", ctx.Product.ID, ctx.Order.ID, model.StockBizOrderRollback).Count(&rollbackLogCountAfterSecondCancel).Error; err != nil {
		t.Fatalf("count rollback stock logs after second cancel failed: %v", err)
	}

	if rollbackLogCountAfterFirstCancel != rollbackLogCountAfterSecondCancel {
		t.Fatalf("expected rollback log count unchanged on second cancel, before=%d after=%d", rollbackLogCountAfterFirstCancel, rollbackLogCountAfterSecondCancel)
	}

	var orderAfterSecondCancel model.Order
	if err := db.First(&orderAfterSecondCancel, ctx.Order.ID).Error; err != nil {
		t.Fatalf("query order after second cancel failed: %v", err)
	}
	if orderAfterSecondCancel.Status != model.OrderStatusCancelled {
		t.Fatalf("expected order status still cancelled after second cancel, got %d", orderAfterSecondCancel.Status)
	}
	if orderAfterSecondCancel.CancelledAt == nil {
		t.Fatalf("expected cancelled_at not nil after second cancel")
	}

}

func TestOrder_ConcurrentTesting_OrderOversold(t *testing.T) {
	const (
		initialStock = int64(10)
		requests     = 20
	)

	testDB, orderSvc := newOrderService(t)
	product := seedProduct(t, testDB, "test concurrent testing order", 100, model.ProductStatusOnSale)

	seedInventory(t, testDB, product.ID, initialStock)

	var wg sync.WaitGroup
	start := make(chan struct{})
	errCh := make(chan error, requests)

	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start

			_, err := orderSvc.CreateOrder(context.Background(), testUserID, request.CreateOrderRequest{
				IdempotencyKey: newIdempotencyKey(),
				Items: []request.CreateOrderItemRequest{
					{
						ProductID: product.ID,
						Quantity:  1,
					},
				},
			})

			errCh <- err
		}()
	}

	close(start)
	wg.Wait()
	close(errCh)

	var successCount, failedCount int64
	for err := range errCh {
		if err == nil {
			successCount++
			continue
		}
		if !errors.Is(err, service.ErrInsufficientStock) {
			t.Fatalf("expected ErrInsufficientStock, got %v", err)
		}
		failedCount++
	}

	if successCount != initialStock {
		t.Fatalf("expected success count %d, got %d", initialStock, successCount)
	}
	if failedCount != requests-initialStock {
		t.Fatalf("expected failed count %d, got %d", requests-initialStock, failedCount)
	}

	var stockLogCount int64
	if err := testDB.Model(&model.StockLog{}).Where("product_id = ? AND biz_type = ?", product.ID, model.StockBizOrderDeduct).Count(&stockLogCount).Error; err != nil {
		t.Fatalf("count stock logs failed: %v", err)
	}

	if stockLogCount != initialStock {
		t.Fatalf("expected stock log count %d,got %d", initialStock, stockLogCount)
	}

	var inventoryFinaly model.Inventory
	if err := testDB.Where("product_id = ?", product.ID).First(&inventoryFinaly).Error; err != nil {
		t.Fatalf("query inventory failed:%v", err)
	}

	if inventoryFinaly.StockQuantity != 0 {
		t.Fatalf("expected inventory quantity is 0,got %d", inventoryFinaly.StockQuantity)
	}

}

func TestOrder_ConcurrentPurchaseMultipleQuantity_NoOversold(t *testing.T) {
	const (
		initialStock   = int64(10)
		deductQuantity = int64(3)
		requests       = int64(5)
	)

	testDB, orderSvc := newOrderService(t)
	product := seedProduct(t, testDB, "test concurrent purchase multiple quantity", 100, model.ProductStatusOnSale)
	seedInventory(t, testDB, product.ID, initialStock)

	var wg sync.WaitGroup
	start := make(chan struct{})
	errCh := make(chan error, requests)

	for i := int64(0); i < requests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start

			_, err := orderSvc.CreateOrder(context.Background(), testUserID, request.CreateOrderRequest{
				IdempotencyKey: newIdempotencyKey(),
				Items: []request.CreateOrderItemRequest{
					{
						ProductID: product.ID,
						Quantity:  deductQuantity,
					},
				},
			})

			errCh <- err
		}()
	}

	close(start)
	wg.Wait()
	close(errCh)

	var successCount, failedCount int64
	for err := range errCh {
		if err == nil {
			successCount++
			continue
		}
		if !errors.Is(err, service.ErrInsufficientStock) {
			t.Fatalf("expected ErrInsufficientStock, got %v", err)
		}
		failedCount++
	}

	expectedSuccess := initialStock / deductQuantity
	if requests < expectedSuccess {
		expectedSuccess = requests
	}
	expectedFailed := requests - expectedSuccess
	expectedRemaining := initialStock - expectedSuccess*deductQuantity

	if successCount != expectedSuccess {
		t.Fatalf("expected success count %d, got %d", expectedSuccess, successCount)
	}
	if failedCount != expectedFailed {
		t.Fatalf("expected failed count %d, got %d", expectedFailed, failedCount)
	}

	var stockLogCount int64
	if err := testDB.Model(&model.StockLog{}).Where("product_id = ? AND biz_type = ?", product.ID, model.StockBizOrderDeduct).Count(&stockLogCount).Error; err != nil {
		t.Fatalf("count stock logs failed: %v", err)
	}

	if stockLogCount != expectedSuccess {
		t.Fatalf("expected stock log count %d, got %d", expectedSuccess, stockLogCount)
	}

	var inventoryFinally model.Inventory
	if err := testDB.Where("product_id = ?", product.ID).First(&inventoryFinally).Error; err != nil {
		t.Fatalf("query inventory failed: %v", err)
	}

	if inventoryFinally.StockQuantity != expectedRemaining {
		t.Fatalf("expected inventory quantity is %d, got %d", expectedRemaining, inventoryFinally.StockQuantity)
	}
}

func TestOrder_ConcurrentTestingCancelToAssignOrder(t *testing.T) {
	const (
		initialStock   = int64(10)
		deductQuantity = int64(10)
		requests       = 5
	)

	testDB, orderStr := newOrderService(t)
	product := seedProduct(t, testDB, "test order concurrent testing cancel to assign order", 100, model.ProductStatusOnSale)
	seedInventory(t, testDB, product.ID, initialStock)

	order, err := orderStr.CreateOrder(context.Background(), testUserID, request.CreateOrderRequest{
		IdempotencyKey: newIdempotencyKey(),
		Items: []request.CreateOrderItemRequest{
			{
				ProductID: product.ID,
				Quantity:  deductQuantity,
			},
		},
	})

	if err != nil {
		t.Fatalf("expected create order success, got %v", err)
	}

	var wg sync.WaitGroup
	start := make(chan struct{})
	errCh := make(chan error, requests)

	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errCh <- orderStr.CancelOrder(context.Background(), testUserID, order.ID)
		}()
	}

	close(start)
	wg.Wait()
	close(errCh)

	var successCount int64
	for err := range errCh {
		if err == nil {
			successCount++
			continue
		}
		if !errors.Is(err, service.ErrOrderCancelFailed) {
			t.Fatalf("expected nil or ErrOrderCancelFailed, got %v", err)
		}
	}

	if successCount == 0 {
		t.Fatalf("expected at least one cancel request success")
	}

	var inventory model.Inventory
	if err := testDB.Where("product_id = ?", product.ID).First(&inventory).Error; err != nil {
		t.Fatalf("query inventory failed: %v", err)
	}

	if inventory.StockQuantity != initialStock {
		t.Fatalf("expected inventory rollback to %d, got %d", initialStock, inventory.StockQuantity)
	}

	var stockLogCount int64
	if err := testDB.Model(&model.StockLog{}).Where("product_id = ? AND biz_type = ?", product.ID, model.StockBizOrderRollback).Count(&stockLogCount).Error; err != nil {
		t.Fatalf("expected query stock log success, got %v", err)
	}

	if stockLogCount != 1 {
		t.Fatalf("expected rollback stock log count is 1, got %d", stockLogCount)
	}

	var cancelledOrder model.Order
	if err := testDB.First(&cancelledOrder, order.ID).Error; err != nil {
		t.Fatalf("query cancelled order failed: %v", err)
	}
	if cancelledOrder.Status != model.OrderStatusCancelled {
		t.Fatalf("expected order status cancelled, got %d", cancelledOrder.Status)
	}
	if cancelledOrder.CancelledAt == nil {
		t.Fatalf("expected cancelled_at not nil")
	}
}

func TestOrder_ConcurrentTestingPayingToAssignOrder(t *testing.T) {
	const requests = int64(5)

	testDB, orderSvc := newOrderService(t)
	ctx := seedPendingOrderContext(t, testDB)

	var wg sync.WaitGroup
	start := make(chan struct{})
	errCh := make(chan error, requests)

	for i := int64(0); i < requests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errCh <- orderSvc.PayOrder(context.Background(), testUserID, ctx.Order.ID)
		}()
	}

	close(start)
	wg.Wait()
	close(errCh)

	var successCount, failedCount int64
	for err := range errCh {
		if err == nil {
			successCount++
			continue
		}
		if !errors.Is(err, service.ErrOrderAlreadyPaid) && !errors.Is(err, service.ErrOrderPayFailed) {
			t.Fatalf("expected ErrOrderAlreadyPaid or ErrOrderPayFailed, got %v", err)
		}
		failedCount++
	}

	if successCount != 1 {
		t.Fatalf("expected success count 1, got %d", successCount)
	}
	if failedCount != requests-1 {
		t.Fatalf("expected failed count %d, got %d", requests-1, failedCount)
	}

	var paidOrder model.Order
	if err := testDB.First(&paidOrder, ctx.Order.ID).Error; err != nil {
		t.Fatalf("query paid order failed: %v", err)
	}
	if paidOrder.Status != model.OrderStatusPaid {
		t.Fatalf("expected order status paid, got %d", paidOrder.Status)
	}
	if paidOrder.PaidAt == nil {
		t.Fatalf("expected paid_at not nil")
	}

	var inventory model.Inventory
	if err := testDB.Where("product_id = ?", ctx.Product.ID).First(&inventory).Error; err != nil {
		t.Fatalf("query inventory failed: %v", err)
	}
	expectedStock := ctx.InitQty - ctx.OrderQty
	if inventory.StockQuantity != expectedStock {
		t.Fatalf("expected inventory unchanged after pay: %d, got %d", expectedStock, inventory.StockQuantity)
	}

	var stockLogCount int64
	if err := testDB.Model(&model.StockLog{}).Where("product_id = ? AND biz_id = ?", ctx.Product.ID, ctx.Order.ID).Count(&stockLogCount).Error; err != nil {
		t.Fatalf("count stock logs failed: %v", err)
	}
	if stockLogCount != 1 {
		t.Fatalf("expected stock log count unchanged after pay: 1, got %d", stockLogCount)
	}
}

func TestOrder_ConcurrentPayAndCancel_OnlyOneFinalState(t *testing.T) {
	testDB, orderSvc := newOrderService(t)
	ctx := seedPendingOrderContext(t, testDB)

	start := make(chan struct{})
	var wg sync.WaitGroup

	var payErr error
	var cancelErr error

	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		payErr = orderSvc.PayOrder(context.Background(), testUserID, ctx.Order.ID)
	}()
	go func() {
		defer wg.Done()
		<-start
		cancelErr = orderSvc.CancelOrder(context.Background(), testUserID, ctx.Order.ID)
	}()

	close(start)
	wg.Wait()

	if payErr != nil && !errors.Is(payErr, service.ErrOrderAlreadyCanceled) && !errors.Is(payErr, service.ErrOrderPayFailed) {
		t.Fatalf("unexpected pay error: %v", payErr)
	}
	if cancelErr != nil && !errors.Is(cancelErr, service.ErrOrderAlreadyPaid) && !errors.Is(cancelErr, service.ErrOrderCancelFailed) {
		t.Fatalf("unexpected cancel error: %v", cancelErr)
	}

	successCount := 0
	if payErr == nil {
		successCount++
	}
	if cancelErr == nil {
		successCount++
	}
	if successCount != 1 {
		t.Fatalf("expected exactly one of pay or cancel to succeed, payErr=%v cancelErr=%v", payErr, cancelErr)
	}

	var got model.Order
	if err := testDB.First(&got, ctx.Order.ID).Error; err != nil {
		t.Fatalf("query order failed: %v", err)
	}

	var inventory model.Inventory
	if err := testDB.Where("product_id = ?", ctx.Product.ID).First(&inventory).Error; err != nil {
		t.Fatalf("query inventory failed: %v", err)
	}

	var rollbackLogCount int64
	if err := testDB.Model(&model.StockLog{}).Where("product_id = ? AND biz_id = ? AND biz_type = ?", ctx.Product.ID, ctx.Order.ID, model.StockBizOrderRollback).Count(&rollbackLogCount).Error; err != nil {
		t.Fatalf("count rollback stock logs failed: %v", err)
	}

	switch got.Status {
	case model.OrderStatusPaid:
		if got.PaidAt == nil {
			t.Fatalf("expected paid_at not nil when final status is paid")
		}
		if got.CancelledAt != nil {
			t.Fatalf("expected cancelled_at nil when final status is paid")
		}
		expectedStock := ctx.InitQty - ctx.OrderQty
		if inventory.StockQuantity != expectedStock {
			t.Fatalf("expected stock remain deducted as %d when paid wins, got %d", expectedStock, inventory.StockQuantity)
		}
		if rollbackLogCount != 0 {
			t.Fatalf("expected no rollback log when paid wins, got %d", rollbackLogCount)
		}
	case model.OrderStatusCancelled:
		if got.CancelledAt == nil {
			t.Fatalf("expected cancelled_at not nil when final status is cancelled")
		}
		if got.PaidAt != nil {
			t.Fatalf("expected paid_at nil when final status is cancelled")
		}
		if inventory.StockQuantity != ctx.InitQty {
			t.Fatalf("expected stock rollback to %d when cancel wins, got %d", ctx.InitQty, inventory.StockQuantity)
		}
		if rollbackLogCount != 1 {
			t.Fatalf("expected one rollback log when cancel wins, got %d", rollbackLogCount)
		}
	default:
		t.Fatalf("expected final status paid or cancelled, got %d", got.Status)
	}
}

func TestCreateOrder_IdempotentReplayReturnsSameOrder(t *testing.T) {
	testDB, orderSvc := newOrderService(t)
	product := seedProduct(t, testDB, "idempotent-replay-product", 100, model.ProductStatusOnSale)
	seedInventory(t, testDB, product.ID, 10)

	req := request.CreateOrderRequest{
		IdempotencyKey: newIdempotencyKey(),
		Items: []request.CreateOrderItemRequest{
			{ProductID: product.ID, Quantity: 2},
		},
	}

	first, err := orderSvc.CreateOrder(context.Background(), testUserID, req)
	if err != nil {
		t.Fatalf("first create order failed: %v", err)
	}
	second, err := orderSvc.CreateOrder(context.Background(), testUserID, req)
	if err != nil {
		t.Fatalf("idempotent replay failed: %v", err)
	}

	if second.ID != first.ID || second.OrderNo != first.OrderNo {
		t.Fatalf("expected replay to return order %d/%s, got %d/%s", first.ID, first.OrderNo, second.ID, second.OrderNo)
	}

	var orderCount int64
	if err := testDB.Model(&model.Order{}).Count(&orderCount).Error; err != nil {
		t.Fatalf("count orders failed: %v", err)
	}
	if orderCount != 1 {
		t.Fatalf("expected one order, got %d", orderCount)
	}

	var inventory model.Inventory
	if err := testDB.Where("product_id = ?", product.ID).First(&inventory).Error; err != nil {
		t.Fatalf("query inventory failed: %v", err)
	}
	if inventory.StockQuantity != 8 {
		t.Fatalf("expected inventory 8 after replay, got %d", inventory.StockQuantity)
	}

	var record model.OrderIdempotencyKey
	if err := testDB.Where("idempotency_key = ?", req.IdempotencyKey).First(&record).Error; err != nil {
		t.Fatalf("query idempotency record failed: %v", err)
	}
	if record.Status != model.OrderAlreadyCreated || record.OrderID == nil || *record.OrderID != first.ID {
		t.Fatalf("unexpected idempotency record: status=%d order_id=%v", record.Status, record.OrderID)
	}
}

func TestCreateOrder_SameIdempotencyKeyDifferentRequest_ReturnsConflict(t *testing.T) {
	testDB, orderSvc := newOrderService(t)
	product := seedProduct(t, testDB, "idempotent-conflict-product", 100, model.ProductStatusOnSale)
	seedInventory(t, testDB, product.ID, 10)
	key := newIdempotencyKey()

	first, err := orderSvc.CreateOrder(context.Background(), testUserID, request.CreateOrderRequest{
		IdempotencyKey: key,
		Items: []request.CreateOrderItemRequest{
			{ProductID: product.ID, Quantity: 1},
		},
	})

	if err != nil {
		t.Fatalf("first create order failed: %v", err)
	}

	_, err = orderSvc.CreateOrder(context.Background(), testUserID, request.CreateOrderRequest{
		IdempotencyKey: key,
		Items: []request.CreateOrderItemRequest{
			{ProductID: product.ID, Quantity: 2},
		},
	})

	if !errors.Is(err, service.ErrOrderIdempotencyConflict) {
		t.Fatalf("expected ErrOrderIdempotencyConflict, got %v", err)
	}

	var orderCount int64
	if err := testDB.Model(&model.Order{}).Count(&orderCount).Error; err != nil {
		t.Fatalf("count orders failed: %v", err)
	}
	if orderCount != 1 {
		t.Fatalf("expected one order after conflict, got %d", orderCount)
	}

	var inventory model.Inventory
	if err := testDB.Where("product_id = ?", product.ID).First(&inventory).Error; err != nil {
		t.Fatalf("query inventory failed: %v", err)
	}
	if inventory.StockQuantity != 9 {
		t.Fatalf("expected inventory 9 after conflict, got %d", inventory.StockQuantity)
	}
	if first.ID == 0 {
		t.Fatal("expected first order ID to be assigned")
	}
}

func TestCreateOrder_ConcurrentSameIdempotencyKey_CreatesOneOrder(t *testing.T) {
	const requests = 12

	testDB, orderSvc := newOrderService(t)
	product := seedProduct(t, testDB, "concurrent-idempotent-product", 100, model.ProductStatusOnSale)
	seedInventory(t, testDB, product.ID, 10)

	req := request.CreateOrderRequest{
		IdempotencyKey: newIdempotencyKey(),
		Items: []request.CreateOrderItemRequest{
			{ProductID: product.ID, Quantity: 1},
		},
	}

	type result struct {
		order *model.Order
		err   error
	}

	start := make(chan struct{})
	results := make(chan result, requests)
	var wg sync.WaitGroup
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			order, err := orderSvc.CreateOrder(context.Background(), testUserID, req)
			results <- result{order: order, err: err}
		}()
	}

	close(start)
	wg.Wait()
	close(results)

	var orderID int64
	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent idempotent create failed: %v", result.err)
		}
		if result.order == nil {
			t.Fatal("concurrent idempotent create returned nil order")
		}
		if orderID == 0 {
			orderID = result.order.ID
			continue
		}
		if result.order.ID != orderID {
			t.Fatalf("expected order ID %d, got %d", orderID, result.order.ID)
		}
	}

	var orderCount int64
	if err := testDB.Model(&model.Order{}).Count(&orderCount).Error; err != nil {
		t.Fatalf("count orders failed: %v", err)
	}
	if orderCount != 1 {
		t.Fatalf("expected one order, got %d", orderCount)
	}

	var stockLogCount int64
	if err := testDB.Model(&model.StockLog{}).
		Where("product_id = ? AND biz_type = ?", product.ID, model.StockBizOrderDeduct).
		Count(&stockLogCount).Error; err != nil {
		t.Fatalf("count stock logs failed: %v", err)
	}
	if stockLogCount != 1 {
		t.Fatalf("expected one stock log, got %d", stockLogCount)
	}

	var inventory model.Inventory
	if err := testDB.Where("product_id = ?", product.ID).First(&inventory).Error; err != nil {
		t.Fatalf("query inventory failed: %v", err)
	}
	if inventory.StockQuantity != 9 {
		t.Fatalf("expected inventory 9, got %d", inventory.StockQuantity)
	}
}

func TestCreateOrder_FailedAttemptRollsBackIdempotencyKey(t *testing.T) {
	testDB, orderSvc := newOrderService(t)
	product := seedProduct(t, testDB, "idempotent-retry-product", 100, model.ProductStatusOnSale)
	seedInventory(t, testDB, product.ID, 1)

	req := request.CreateOrderRequest{
		IdempotencyKey: newIdempotencyKey(),
		Items: []request.CreateOrderItemRequest{
			{ProductID: product.ID, Quantity: 2},
		},
	}

	_, err := orderSvc.CreateOrder(context.Background(), testUserID, req)
	if !errors.Is(err, service.ErrInsufficientStock) {
		t.Fatalf("expected ErrInsufficientStock, got %v", err)
	}

	var recordCount int64
	if err := testDB.Model(&model.OrderIdempotencyKey{}).
		Where("idempotency_key = ?", req.IdempotencyKey).
		Count(&recordCount).Error; err != nil {
		t.Fatalf("count idempotency records failed: %v", err)
	}
	if recordCount != 0 {
		t.Fatalf("expected failed attempt to roll back idempotency key, got %d records", recordCount)
	}

	if err := testDB.Model(&model.Inventory{}).
		Where("product_id = ?", product.ID).
		Update("stock_quantity", 2).Error; err != nil {
		t.Fatalf("restore inventory failed: %v", err)
	}

	order, err := orderSvc.CreateOrder(context.Background(), testUserID, req)
	if err != nil {
		t.Fatalf("retry create order failed: %v", err)
	}
	if order == nil || order.ID == 0 {
		t.Fatal("expected retry to create an order")
	}
}

func TestCreateOrder_EmptyIdempotencyKey_ReturnsError(t *testing.T) {
	_, orderSvc := newOrderService(t)

	_, err := orderSvc.CreateOrder(context.Background(), testUserID, request.CreateOrderRequest{
		Items: []request.CreateOrderItemRequest{
			{ProductID: 1, Quantity: 1},
		},
	})

	if !errors.Is(err, service.ErrInvalidIdempotencyKey) {
		t.Fatalf("expected ErrInvalidIdempotencyKey, got %v", err)
	}
}
