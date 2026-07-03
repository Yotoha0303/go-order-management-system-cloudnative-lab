package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"go-order-management-system/internal/dao"
	"go-order-management-system/internal/model"
	"go-order-management-system/internal/request"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	orderNoPrefix    = "ORD"
	maxOrderPage     = 1_000_000
	maxOrderPageSize = 100
)

func (p *OrderService) CreateOrder(ctx context.Context, userID int64, req request.CreateOrderRequest) (*model.Order, error) {
	if userID <= 0 {
		return nil, ErrInvalidUserID
	}
	if strings.TrimSpace(req.IdempotencyKey) == "" || len(req.IdempotencyKey) > 128 {
		return nil, ErrInvalidIdempotencyKey
	}

	requestHash, err := buildCreateOrderRequestHash(req)
	if err != nil {
		return nil, err
	}

	var createOrder *model.Order
	err = p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		acquired, err := dao.TryCreateOrderIdempotencyKey(tx, ctx, userID, req.IdempotencyKey, requestHash)
		if err != nil {
			return err
		}
		if !acquired {
			record, err := dao.GetOrderIdempotencyKey(tx, ctx, userID, req.IdempotencyKey)
			if err != nil {
				return err
			}
			if record.RequestHash != requestHash {
				return ErrOrderIdempotencyConflict
			}

			switch record.Status {
			case model.OrderAlreadyCreated:
				if record.OrderID == nil || *record.OrderID <= 0 {
					return ErrOrderIdempotencyStateInvalid
				}

				createOrder, err = dao.GetOrderByID(ctx, tx, userID, *record.OrderID)
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrOrderIdempotencyStateInvalid
				}
				return err
			case model.OrderBeingCreated:
				return ErrOrderBeingCreated
			default:
				return ErrOrderIdempotencyStateInvalid
			}
		}

		var totalAmountFen int64
		order := &model.Order{
			UserID:         userID,
			OrderNo:        generateOrderNo(),
			TotalAmountFen: 0,
			Status:         model.OrderStatusPending,
		}
		if err := dao.CreateOrder(ctx, tx, order); err != nil {
			return err
		}

		if err := validateDuplicateItems(req.Items); err != nil {
			return err
		}

		for _, itemReq := range req.Items {
			product, err := dao.GetProductByID(ctx, tx, itemReq.ProductID)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrProductNotFound
				}
				return err
			}
			if product.Status != model.ProductStatusOnSale {
				return ErrProductOffSale
			}

			inv, err := dao.GetInventoryByProductIDForUpdate(ctx, tx, itemReq.ProductID)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrInventoryNotFound
				}
				return err
			}

			beforeQuantity := inv.StockQuantity
			afterQuantity := beforeQuantity - itemReq.Quantity
			if afterQuantity < 0 {
				return ErrInsufficientStock
			}

			rows, err := dao.DeductInventory(ctx, tx, itemReq.ProductID, itemReq.Quantity)
			if err != nil {
				return err
			}
			if rows == 0 {
				return ErrInsufficientStock
			}

			subtotalFen := product.PriceFen * itemReq.Quantity
			totalAmountFen += subtotalFen

			orderItem := &model.OrderItem{
				OrderID:         order.ID,
				ProductID:       product.ID,
				ProductName:     product.Name,
				ProductPriceFen: product.PriceFen,
				Quantity:        itemReq.Quantity,
				SubtotalFen:     subtotalFen,
			}
			if err := dao.CreateOrderItem(ctx, tx, orderItem); err != nil {
				return err
			}

			stockLog := &model.StockLog{
				ProductID:      product.ID,
				ChangeQuantity: -itemReq.Quantity,
				BeforeQuantity: beforeQuantity,
				AfterQuantity:  afterQuantity,
				BizType:        model.StockBizOrderDeduct,
				BizID:          &order.ID,
				Remark:         "创建订单扣减库存：" + order.OrderNo,
			}
			if err := dao.CreateStockLog(ctx, tx, stockLog); err != nil {
				return ErrCreateStockLogFailed
			}
		}

		if err := dao.PatchOrderTotalPriceFen(ctx, tx, order.ID, totalAmountFen, userID); err != nil {
			return err
		}

		rowsAffected, err := dao.CompleteOrderIdempotencyKey(tx, ctx, userID, req.IdempotencyKey, order.ID)
		if err != nil || rowsAffected != 1 {
			return ErrOrderIdempotencyStateInvalid
		}

		order.TotalAmountFen = totalAmountFen
		createOrder = order
		return nil
	})
	if err != nil {
		return nil, err
	}

	return createOrder, nil
}

func validateDuplicateItems(items []request.CreateOrderItemRequest) error {
	seen := make(map[int64]struct{}, len(items))

	for _, item := range items {
		if _, ok := seen[item.ProductID]; ok {
			return ErrDuplicateOrderItem
		}
		seen[item.ProductID] = struct{}{}
	}
	return nil
}

func buildCreateOrderRequestHash(req request.CreateOrderRequest) (string, error) {
	items := append([]request.CreateOrderItemRequest(nil), req.Items...)
	sort.Slice(items, func(i, j int) bool {
		if items[i].ProductID == items[j].ProductID {
			return items[i].Quantity < items[j].Quantity
		}
		return items[i].ProductID < items[j].ProductID
	})

	payload := struct {
		Version int                              `json:"version"`
		Items   []request.CreateOrderItemRequest `json:"items"`
	}{
		Version: 1,
		Items:   items,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

func generateOrderNo() string {
	return orderNoPrefix + uuid.NewString()
}

func (p *OrderService) GetOrderByID(ctx context.Context, userID, id int64) (*model.Order, []*model.OrderItem, error) {
	order, err := dao.GetOrderByID(ctx, p.db, userID, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrOrderNotFound
		}
		return nil, nil, err
	}

	items, err := dao.ListOrderItemsByOrderID(ctx, p.db, id)
	if err != nil {
		return nil, nil, err
	}

	return order, items, nil
}

func (p *OrderService) ListOrders(ctx context.Context, userID int64, page, pageSize int) ([]*model.Order, int64, error) {
	if userID <= 0 {
		return nil, 0, ErrInvalidUserID
	}
	if page <= 0 || page > maxOrderPage || pageSize <= 0 || pageSize > maxOrderPageSize {
		return nil, 0, ErrInvalidOrderPagination
	}

	offset := (page - 1) * pageSize
	return dao.ListOrders(ctx, p.db, userID, pageSize, offset)
}

func (p *OrderService) CancelOrder(ctx context.Context, userID, orderID int64) error {
	return p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {

		order, err := dao.GetOrderByID(ctx, tx, userID, orderID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrOrderNotFound
			}
			return err
		}

		switch order.Status {
		case model.OrderStatusCancelled:
			return nil
		case model.OrderStatusPending:
		case model.OrderStatusPaid:
			return ErrOrderAlreadyPaid
		case model.OrderStatusFinished:
			return ErrOrderAlreadyFinished
		default:
			return ErrOrderCancelFailed
		}

		rows, err := dao.PatchOrderStatus(ctx, tx, userID, order.ID, model.OrderStatusPending, model.OrderStatusCancelled, "cancelled_at")
		if err != nil {
			return err
		}

		if rows == 0 {
			return ErrOrderCancelFailed
		}

		items, err := dao.ListOrderItemsByOrderID(ctx, tx, order.ID)
		if err != nil {
			return err
		}

		for _, item := range items {
			inventory, err := dao.GetInventoryByProductIDForUpdate(ctx, tx, item.ProductID)
			if err != nil {
				return err
			}

			before := inventory.StockQuantity
			after := before + item.Quantity

			if err := dao.UpdateInventoryStockQuantity(ctx, tx, item.ProductID, after); err != nil {
				return err
			}

			stockLog := &model.StockLog{
				ProductID:      item.ProductID,
				BizID:          &order.ID,
				ChangeQuantity: item.Quantity,
				AfterQuantity:  after,
				BeforeQuantity: before,
				BizType:        model.StockBizOrderRollback,
				Remark:         "取消订单回滚库存：" + order.OrderNo,
			}

			if err := dao.CreateStockLog(ctx, tx, stockLog); err != nil {
				return err
			}
		}

		return nil
	})
}

func (p *OrderService) PayOrder(ctx context.Context, userID, orderID int64) error {
	order, err := dao.GetOrderByID(ctx, p.db, userID, orderID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrOrderNotFound
		}
		return err
	}

	switch order.Status {
	case model.OrderStatusPaid:
		return ErrOrderAlreadyPaid
	case model.OrderStatusFinished:
		return ErrOrderAlreadyFinished
	case model.OrderStatusCancelled:
		return ErrOrderAlreadyCanceled
	case model.OrderStatusPending:
	default:
		return ErrOrderPayFailed
	}

	row, err := dao.PatchOrderStatus(ctx, p.db, userID, order.ID, model.OrderStatusPending, model.OrderStatusPaid, "paid_at")
	if err != nil {
		return err
	}

	if row == 0 {
		return ErrOrderPayFailed
	}
	return nil
}

func (p *OrderService) FinishOrder(ctx context.Context, userID, orderID int64) error {
	order, err := dao.GetOrderByID(ctx, p.db, userID, orderID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrOrderNotFound
		}
		return err
	}

	switch order.Status {
	case model.OrderStatusPending:
		return ErrOrderNotPaid
	case model.OrderStatusCancelled:
		return ErrOrderAlreadyCanceled
	case model.OrderStatusFinished:
		return ErrOrderAlreadyFinished
	case model.OrderStatusPaid:
	default:
		return ErrOrderFinishFailed
	}

	row, err := dao.PatchOrderStatus(ctx, p.db, userID, order.ID, model.OrderStatusPaid, model.OrderStatusFinished, "completed_at")
	if err != nil {
		return err
	}

	if row == 0 {
		return ErrOrderFinishFailed
	}
	return nil

}
