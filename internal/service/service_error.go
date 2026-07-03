package service

import (
	"go-order-management-system/internal/apperror"
	"go-order-management-system/internal/response"
	"net/http"
)

// product_inventory err
var (
	ErrInitInventoryFailed = apperror.New(
		http.StatusInternalServerError,
		response.CodeInitInventoryFailed,
		"初始化库存失败",
	)
	ErrInitInventoryExists = apperror.New(
		http.StatusConflict,
		response.CodeInitInventoryExists,
		"库存已初始化",
	)
	ErrInventoryNotFound = apperror.New(
		http.StatusNotFound,
		response.CodeInventoryNotFound,
		"库存未找到",
	)
	ErrInvalidAddQuantity = apperror.New(
		http.StatusBadRequest,
		response.CodeInventoryInvalidQuantity,
		"增加的库存数量必须大于0",
	)

	ErrInvalidStockQuantity = apperror.New(
		http.StatusBadRequest,
		response.CodeParameterError,
		"库存数量不能为负",
	)
)

// product err
var (
	ErrInvalidProductPrice = apperror.New(
		http.StatusBadRequest,
		response.CodeParameterError,
		"价格必须大于0",
	)

	ErrInvalidProductName = apperror.New(
		http.StatusBadRequest,
		response.CodeParameterError,
		"名称不能为空",
	)

	ErrInvalidProductDescription = apperror.New(
		http.StatusBadRequest,
		response.CodeParameterError,
		"描述不能超过500个字符",
	)

	ErrProductNotFound = apperror.New(
		http.StatusNotFound,
		response.CodeProductNotFound,
		"商品信息不存在",
	)

	ErrInvalidProductID = apperror.New(
		http.StatusBadRequest,
		response.CodeParameterError,
		"无效的商品ID",
	)

	ErrProductOnSaleFailed = apperror.New(
		http.StatusConflict,
		response.CodeProductOnSaleFailed,
		"上架商品失败",
	)

	ErrProductOffSaleFailed = apperror.New(
		http.StatusConflict,
		response.CodeProductOffSaleFailed,
		"下架商品失败",
	)
)

// stock log err
var (
	ErrCreateStockLogFailed = apperror.New(
		http.StatusNotFound,
		response.CodeCreateStockLogFailed,
		"创建库存日志失败",
	)
)

var (
	ErrInvalidIdempotencyKey = apperror.New(
		http.StatusBadRequest,
		response.CodeOrderParameterError,
		"idempotency key 不能为空且长度不能超过 128",
	)
	ErrOrderIdempotencyConflict = apperror.New(
		http.StatusConflict,
		response.CodeOrderIdempotencyConflict,
		"相同 idempotency key 不能用于不同的创建订单请求",
	)
	ErrOrderBeingCreated = apperror.New(
		http.StatusConflict,
		response.CodeOrderBeingCreated,
		"订单正在创建中",
	)
	ErrOrderIdempotencyStateInvalid = apperror.New(
		http.StatusInternalServerError,
		response.CodeOrderIdempotencyStateError,
		"订单幂等记录状态异常",
	)

	ErrDuplicateOrderItem = apperror.New(
		http.StatusBadRequest,
		response.CodeParameterError,
		"订单中的商品重复",
	)
)

// order err
var (
	ErrInvalidOrderPagination = apperror.New(
		http.StatusBadRequest,
		response.CodeOrderParameterError,
		"分页参数无效",
	)

	ErrProductOffSale = apperror.New(
		http.StatusConflict,
		response.CodeProductAlreadyOffSale,
		"商品已下架",
	)

	ErrInsufficientStock = apperror.New(
		http.StatusConflict,
		response.CodeInsufficientStock,
		"库存不足",
	)

	ErrCreateOrderFailed = apperror.New(
		http.StatusInternalServerError,
		response.CodeCreateOrderFailed,
		"创建订单失败",
	)

	ErrOrderNotFound = apperror.New(
		http.StatusNotFound,
		response.CodeOrderNotFound,
		"订单不存在",
	)

	ErrOrderPayFailed = apperror.New(
		http.StatusConflict,
		response.CodeOrderPayFailed,
		"订单支付失败",
	)

	ErrOrderFinishFailed = apperror.New(
		http.StatusConflict,
		response.CodeOrderFinishFailed,
		"订单完成失败",
	)

	ErrOrderCancelFailed = apperror.New(
		http.StatusConflict,
		response.CodeOrderCancelFailed,
		"订单取消失败",
	)

	ErrOrderNotPaid = apperror.New(
		http.StatusConflict,
		response.CodeOrderNotPaid,
		"订单未支付",
	)

	ErrOrderAlreadyCanceled = apperror.New(
		http.StatusConflict,
		response.CodeOrderAlreadyCanceled,
		"订单已取消",
	)

	ErrOrderAlreadyFinished = apperror.New(
		http.StatusConflict,
		response.CodeOrderAlreadyFinished,
		"订单已完成",
	)

	ErrOrderAlreadyPaid = apperror.New(
		http.StatusConflict,
		response.CodeOrderAlreadyPaid,
		"订单已支付",
	)
)
