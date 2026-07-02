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

type InventoryService interface {
	InitInventory(ctx context.Context, req *request.InitInventoryRequest) error
	AddInventory(ctx context.Context, req request.AddInventoryRequest) error
	GetInventoryByProductID(ctx context.Context, productID int64) (*model.Inventory, error)
}

type InventoryHandler struct {
	inventoryService InventoryService
}

func NewInventoryHandler(inventoryService InventoryService) *InventoryHandler {
	return &InventoryHandler{
		inventoryService: inventoryService,
	}
}

var _ InventoryService = (*service.InventoryService)(nil)

func (p *InventoryHandler) InitInventory(c *gin.Context) {
	var req request.InitInventoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, response.CodeParameterError, "请求参数错误")
		return
	}

	if err := p.inventoryService.InitInventory(c.Request.Context(), &req); err != nil {
		handleError(c, err, response.CodeInitInventoryFailed, "初始化库存错误")
		return
	}

	response.Success(c, nil)
}

func (p *InventoryHandler) AddInventory(c *gin.Context) {
	var req request.AddInventoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, response.CodeParameterError, "请求参数错误")
		return
	}

	if err := p.inventoryService.AddInventory(c.Request.Context(), req); err != nil {
		handleError(c, err, response.CodeAddInventoryError, "添加库存失败")
		return
	}

	response.Success(c, nil)
}

func (p *InventoryHandler) GetInventoryByProductID(c *gin.Context) {
	id, ok := parsePositiveID(c, "product_id")
	if !ok {
		return
	}

	inventory, err := p.inventoryService.GetInventoryByProductID(c.Request.Context(), id)
	if err != nil {
		handleError(c, err, response.CodeInventoryNotFound, "查询库存失败")
		return
	}

	response.Success(c, inventory)
}
