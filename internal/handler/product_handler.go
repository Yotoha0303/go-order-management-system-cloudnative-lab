package handler

import (
	"context"
	"go-order-management-system/internal/model"
	"go-order-management-system/internal/request"
	"go-order-management-system/internal/response"
	"go-order-management-system/internal/service"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type ProductService interface {
	CreateProduct(ctx context.Context, req request.CreateProductRequest) (*model.Product, error)
	ListProducts(ctx context.Context, req request.ListProductsRequest) ([]*model.Product, int64, error)
	GetProductByID(ctx context.Context, id int64) (*model.Product, error)
	OnSaleProduct(ctx context.Context, id int64) error
	OffSaleProduct(ctx context.Context, id int64) error
}

type ProductHandler struct {
	productService ProductService
}

func NewProductHandler(productService ProductService) *ProductHandler {
	return &ProductHandler{
		productService: productService,
	}
}

var _ ProductService = (*service.ProductService)(nil)

func (p *ProductHandler) CreateProduct(c *gin.Context) {
	var req request.CreateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, response.CodeParameterError, "参数错误")
		return
	}

	product, err := p.productService.CreateProduct(c.Request.Context(), req)

	if err != nil {
		handleError(c, err, response.CodeCreateProductFailed, "创建商品失败")
		return
	}

	response.Success(c, product)
}

func (p *ProductHandler) ListProducts(c *gin.Context) {
	status, ok := parseProductStatus(c.Query("status"))
	if !ok {
		response.Fail(c, http.StatusBadRequest, response.CodeParameterError, "status 必须是 1、2 或 all")
		return
	}

	page, pageSize, ok := parseOptionalPagination(c.Query("page"), c.Query("page_size"))
	if !ok {
		response.Fail(c, http.StatusBadRequest, response.CodeParameterError, "分页参数错误")
		return
	}

	products, total, err := p.productService.ListProducts(c.Request.Context(), request.ListProductsRequest{
		Status: status, Page: page, PageSize: pageSize,
	})

	if err != nil {
		handleError(c, err, response.CodeQueryProductListFailed, "查询商品列表失败")
		return
	}

	if pageSize > 0 {
		response.Success(c, response.ProductListResponse{
			Products: products,
			Total:    total,
			Page:     page,
			PageSize: pageSize,
		})
		return
	}
	response.Success(c, products)
}

func parseOptionalPagination(pageValue, pageSizeValue string) (int, int, bool) {
	if pageValue == "" && pageSizeValue == "" {
		return 0, 0, true
	}

	page, pageSize := 1, 20
	var err error
	if pageValue != "" {
		page, err = strconv.Atoi(pageValue)
		if err != nil {
			return 0, 0, false
		}
	}
	if pageSizeValue != "" {
		pageSize, err = strconv.Atoi(pageSizeValue)
		if err != nil {
			return 0, 0, false
		}
	}
	if page <= 0 || pageSize <= 0 || pageSize > 100 {
		return 0, 0, false
	}
	return page, pageSize, true
}

func parseProductStatus(value string) (*int8, bool) {
	if value == "all" {
		return nil, true
	}
	if value == "" {
		status := model.ProductStatusOffSale
		return &status, true
	}

	parsed, err := strconv.ParseInt(value, 10, 8)
	if err != nil || (parsed != int64(model.ProductStatusOnSale) && parsed != int64(model.ProductStatusOffSale)) {
		return nil, false
	}
	status := int8(parsed)
	return &status, true
}

func parsePositiveID(c *gin.Context, paramName string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(paramName), 10, 64)
	if err != nil || id <= 0 {
		response.Fail(c, http.StatusBadRequest, response.CodeParameterError, "请求参数错误")
		return 0, false
	}
	return id, true
}

func (p *ProductHandler) GetProductByID(c *gin.Context) {
	id, ok := parsePositiveID(c, "id")
	if !ok {
		return
	}
	product, err := p.productService.GetProductByID(c.Request.Context(), id)
	if err != nil {
		handleError(c, err, response.CodeQueryProductFailed, "请求商品详情失败")
		return
	}
	response.Success(c, product)
}

func (p *ProductHandler) OnSaleProduct(c *gin.Context) {

	id, ok := parsePositiveID(c, "id")
	if !ok {
		return
	}
	if err := p.productService.OnSaleProduct(c.Request.Context(), id); err != nil {
		handleError(c, err, response.CodeProductOnSaleFailed, "上架商品失败")
		return
	}
	response.Success(c, nil)
}

func (p *ProductHandler) OffSaleProduct(c *gin.Context) {
	id, ok := parsePositiveID(c, "id")
	if !ok {
		return
	}
	if err := p.productService.OffSaleProduct(c.Request.Context(), id); err != nil {
		handleError(c, err, response.CodeProductOffSaleFailed, "下架商品失败")
		return
	}
	response.Success(c, nil)
}
