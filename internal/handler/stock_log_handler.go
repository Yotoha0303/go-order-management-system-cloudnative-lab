package handler

import (
	"context"
	"go-order-management-system/internal/model"
	"go-order-management-system/internal/response"
	"go-order-management-system/internal/service"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type StockLogService interface {
	ListStockLogsByProductID(ctx context.Context, productID *int64) ([]*model.StockLog, error)
}

type StockLogHandler struct {
	stockLogService StockLogService
}

func NewStockLogHandler(stockLogService StockLogService) *StockLogHandler {
	return &StockLogHandler{
		stockLogService: stockLogService,
	}
}

var _ StockLogService = (*service.StockLogService)(nil)

func (p *StockLogHandler) ListStockLogs(c *gin.Context) {
	var productID *int64

	productIDStr := c.Query("product_id")
	if productIDStr != "" {
		id, err := strconv.ParseInt(productIDStr, 10, 64)
		if err != nil || id <= 0 {
			response.Fail(c, http.StatusBadRequest, response.CodeParameterError, "无效的产品ID")
			return
		}
		productID = &id
	}

	stockLogs, err := p.stockLogService.ListStockLogsByProductID(c.Request.Context(), productID)
	if err != nil {
		handleError(c, err, response.CodeQueryStockLogFailed, "库存流水日志失败")
		return
	}

	response.Success(c, stockLogs)
}
