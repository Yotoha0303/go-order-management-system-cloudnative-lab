package catalogsvc

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go-order-management-system/internal/auth"
	"go-order-management-system/internal/middleware"
	"go-order-management-system/internal/platform/internalapi"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	ProductStatusOnSale  int8 = 1
	ProductStatusOffSale int8 = 2
)

var ErrProductNotFound = errors.New("product not found")

type Product struct {
	ID          int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Name        string    `gorm:"type:varchar(100);not null" json:"name"`
	Description string    `gorm:"type:varchar(500);not null;default:''" json:"description"`
	PriceFen    int64     `gorm:"column:price_fen;type:bigint;not null" json:"price_fen"`
	Status      int8      `gorm:"type:tinyint;not null;default:2;index:idx_catalog_products_status" json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (Product) TableName() string { return "catalog_products" }

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service { return &Service{db: db} }

func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(&Product{})
}

func (s *Service) Create(name, description string, priceFen int64) (*Product, error) {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	if name == "" || len(name) > 100 || len(description) > 500 || priceFen <= 0 {
		return nil, errors.New("invalid product")
	}
	product := &Product{
		Name:        name,
		Description: description,
		PriceFen:    priceFen,
		Status:      ProductStatusOffSale,
	}
	if err := s.db.Create(product).Error; err != nil {
		return nil, err
	}
	return product, nil
}

func (s *Service) Get(id int64) (*Product, error) {
	var product Product
	if err := s.db.First(&product, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrProductNotFound
		}
		return nil, err
	}
	return &product, nil
}

func (s *Service) List(status *int8, page, pageSize int) ([]Product, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	query := s.db.Model(&Product{})
	if status != nil {
		query = query.Where("status = ?", *status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var products []Product
	if err := query.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&products).Error; err != nil {
		return nil, 0, err
	}
	return products, total, nil
}

func (s *Service) SetStatus(id int64, status int8) error {
	result := s.db.Model(&Product{}).Where("id = ?", id).Update("status", status)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrProductNotFound
	}
	return nil
}

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func RegisterRoutes(router *gin.Engine, tokenManager *auth.TokenManager, roleChecker middleware.RoleChecker, internalToken string, service *Service) {
	h := NewHandler(service)

	api := router.Group("/api/v1")
	api.Use(middleware.AuthMiddleware(tokenManager))
	api.GET("/products", h.list)
	api.GET("/products/:id", h.get)

	admin := api.Group("")
	admin.Use(middleware.AdminMiddleware(roleChecker))
	admin.POST("/products", h.create)
	admin.PATCH("/products/:id/on-sale", h.onSale)
	admin.PATCH("/products/:id/off-sale", h.offSale)

	internal := router.Group("/internal/v1")
	internal.Use(internalapi.Middleware(internalToken))
	internal.GET("/products/:id", h.internalGet)
}

func (h *Handler) create(c *gin.Context) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		PriceFen    int64  `json:"price_fen"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, 40001, "invalid request")
		return
	}
	product, err := h.service.Create(req.Name, req.Description, req.PriceFen)
	if err != nil {
		fail(c, http.StatusBadRequest, 40002, err.Error())
		return
	}
	ok(c, http.StatusCreated, product)
}

func (h *Handler) list(c *gin.Context) {
	page := parsePositiveInt(c.Query("page"), 1)
	pageSize := parsePositiveInt(c.Query("page_size"), 20)
	var status *int8
	if raw := strings.TrimSpace(c.Query("status")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 8)
		if err != nil {
			fail(c, http.StatusBadRequest, 40003, "invalid status")
			return
		}
		value := int8(parsed)
		status = &value
	}
	products, total, err := h.service.List(status, page, pageSize)
	if err != nil {
		fail(c, http.StatusInternalServerError, 50001, "list products failed")
		return
	}
	ok(c, http.StatusOK, gin.H{
		"list":      products,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func (h *Handler) get(c *gin.Context) {
	id, okID := parseID(c.Param("id"))
	if !okID {
		fail(c, http.StatusBadRequest, 40004, "invalid product id")
		return
	}
	product, err := h.service.Get(id)
	if err != nil {
		if errors.Is(err, ErrProductNotFound) {
			fail(c, http.StatusNotFound, 40401, "product not found")
			return
		}
		fail(c, http.StatusInternalServerError, 50002, "get product failed")
		return
	}
	ok(c, http.StatusOK, product)
}

func (h *Handler) internalGet(c *gin.Context) {
	id, okID := parseID(c.Param("id"))
	if !okID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid product id"})
		return
	}
	product, err := h.service.Get(id)
	if err != nil {
		if errors.Is(err, ErrProductNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "product not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "get product failed"})
		return
	}
	c.JSON(http.StatusOK, product)
}

func (h *Handler) onSale(c *gin.Context)  { h.setStatus(c, ProductStatusOnSale) }
func (h *Handler) offSale(c *gin.Context) { h.setStatus(c, ProductStatusOffSale) }

func (h *Handler) setStatus(c *gin.Context, status int8) {
	id, okID := parseID(c.Param("id"))
	if !okID {
		fail(c, http.StatusBadRequest, 40004, "invalid product id")
		return
	}
	if err := h.service.SetStatus(id, status); err != nil {
		if errors.Is(err, ErrProductNotFound) {
			fail(c, http.StatusNotFound, 40401, "product not found")
			return
		}
		fail(c, http.StatusInternalServerError, 50003, "update product status failed")
		return
	}
	ok(c, http.StatusOK, gin.H{"id": id, "status": status})
}

func parseID(raw string) (int64, bool) {
	id, err := strconv.ParseInt(raw, 10, 64)
	return id, err == nil && id > 0
}

func parsePositiveInt(raw string, fallback int) int {
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func ok(c *gin.Context, status int, data any) {
	c.JSON(status, gin.H{"code": 0, "msg": "success", "data": data})
}

func fail(c *gin.Context, status, code int, message string) {
	c.JSON(status, gin.H{"code": code, "msg": message})
}
