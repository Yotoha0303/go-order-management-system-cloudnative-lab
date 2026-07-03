package handler

import (
	"context"
	"go-order-management-system/internal/model"
	"go-order-management-system/internal/request"
	"go-order-management-system/internal/response"
	"go-order-management-system/internal/service"
	"net/http"

	"github.com/gin-gonic/gin"
)

type OrderService interface {
	CreateOrder(ctx context.Context, userID int64, req request.CreateOrderRequest) (*model.Order, error)
	ListOrders(ctx context.Context, userID int64, page, pageSize int) ([]*model.Order, int64, error)
	GetOrderByID(ctx context.Context, userID, id int64) (*model.Order, []*model.OrderItem, error)
	PayOrder(ctx context.Context, userID, orderID int64) error
	FinishOrder(ctx context.Context, userID, orderID int64) error
	CancelOrder(ctx context.Context, userID, orderID int64) error
}

type OrderHandler struct {
	orderService OrderService
}

func NewOrderHandler(orderService OrderService) *OrderHandler {
	return &OrderHandler{
		orderService: orderService,
	}
}

var _ OrderService = (*service.OrderService)(nil)

func (p *OrderHandler) CreateOrder(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req request.CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, response.CodeOrderParameterError, "请求参数错误")
		return
	}

	if req.IdempotencyKey == "" {
		response.Fail(c, http.StatusBadRequest, response.CodeOrderParameterError, "idempotency key 不能为空")
		return
	}

	order, err := p.orderService.CreateOrder(c.Request.Context(), userID, req)
	if err != nil {
		handleError(c, err, response.CodeCreateOrderFailed, "订单创建失败")
		return
	}

	response.Success(c, order)
}

func (p *OrderHandler) ListOrders(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	req := request.ListOrderRequest{
		Page:     1,
		PageSize: 10,
	}
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, response.CodeOrderParameterError, "请求参数错误")
		return
	}

	orders, total, err := p.orderService.ListOrders(c.Request.Context(), userID, req.Page, req.PageSize)
	if err != nil {
		handleError(c, err, response.CodeQueryOrderListFailed, "查询订单列表失败")
		return
	}

	response.Success(c, response.OrderListResponse{
		Orders:   orders,
		Total:    total,
		Page:     req.Page,
		PageSize: req.PageSize,
	})
}

func (p *OrderHandler) GetOrderByID(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	id, ok := parsePositiveID(c, "id")
	if !ok {
		return
	}

	order, orderItems, err := p.orderService.GetOrderByID(c.Request.Context(), userID, id)
	if err != nil {
		handleError(c, err, response.CodeQueryOrderDetailFailed, "查询订单详情失败")
		return
	}

	orderDetail := response.OrderDetailResponse{
		Order: order,
		Items: orderItems,
	}

	response.Success(c, orderDetail)
}

func (p *OrderHandler) PayOrder(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	orderID, ok := parsePositiveID(c, "id")
	if !ok {
		return
	}

	if err := p.orderService.PayOrder(c.Request.Context(), userID, orderID); err != nil {
		handleError(c, err, response.CodeOrderPayFailed, "支付订单失败")
		return
	}

	response.Success(c, nil)
}

func (p *OrderHandler) FinishOrder(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	orderID, ok := parsePositiveID(c, "id")
	if !ok {
		return
	}

	if err := p.orderService.FinishOrder(c.Request.Context(), userID, orderID); err != nil {
		handleError(c, err, response.CodeOrderFinishFailed, "完成订单失败")
		return
	}

	response.Success(c, nil)
}

func (p *OrderHandler) CancelOrders(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	orderID, ok := parsePositiveID(c, "id")
	if !ok {
		return
	}

	if err := p.orderService.CancelOrder(c.Request.Context(), userID, orderID); err != nil {
		handleError(c, err, response.CodeOrderCancelFailed, "取消订单失败")
		return
	}

	response.Success(c, nil)
}
