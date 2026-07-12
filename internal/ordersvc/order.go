package ordersvc

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"go-order-management-system/internal/auth"
	"go-order-management-system/internal/middleware"
	"go-order-management-system/internal/platform/internalapi"
	platformtelemetry "go-order-management-system/internal/platform/telemetry"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"gorm.io/gorm"
)

const (
	OrderStatusReserving              = "reserving"
	OrderStatusPending                = "pending"
	OrderStatusPaying                 = "paying"
	OrderStatusPaid                   = "paid"
	OrderStatusCancelling             = "cancelling"
	OrderStatusCancelled              = "cancelled"
	OrderStatusFinished               = "finished"
	OrderStatusFailed                 = "failed"
	OrderStatusReconciliationRequired = "reconciliation_required"

	OutboxPending   = "pending"
	OutboxPublished = "published"
	OutboxCompleted = "completed"
	OutboxFailed    = "failed"
)

var (
	ErrOrderNotFound        = errors.New("order not found")
	ErrInvalidOrderRequest  = errors.New("invalid order request")
	ErrInvalidOrderState    = errors.New("invalid order state")
	ErrProductUnavailable   = errors.New("product unavailable")
	ErrReservationFailed    = errors.New("inventory reservation failed")
	ErrCompensationRequired = errors.New("manual reconciliation required")
)

type Order struct {
	ID             int64       `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID         int64       `gorm:"column:user_id;not null;index;uniqueIndex:uk_order_user_idempotency,priority:1" json:"user_id"`
	Status         string      `gorm:"type:varchar(32);not null;index" json:"status"`
	TotalFen       int64       `gorm:"column:total_fen;type:bigint;not null" json:"total_fen"`
	ReservationID  string      `gorm:"column:reservation_id;type:varchar(36);not null;default:'';index" json:"reservation_id"`
	IdempotencyKey string      `gorm:"column:idempotency_key;type:varchar(64);not null;uniqueIndex:uk_order_user_idempotency,priority:2" json:"idempotency_key"`
	FailureReason  string      `gorm:"column:failure_reason;type:varchar(500);not null;default:''" json:"failure_reason,omitempty"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
	Items          []OrderItem `gorm:"foreignKey:OrderID" json:"items,omitempty"`
}

func (Order) TableName() string { return "orders_v2" }

type OrderItem struct {
	ID          uint64 `gorm:"primaryKey;autoIncrement" json:"id"`
	OrderID     int64  `gorm:"column:order_id;not null;index" json:"order_id"`
	ProductID   int64  `gorm:"column:product_id;not null;index" json:"product_id"`
	ProductName string `gorm:"column:product_name;type:varchar(100);not null" json:"product_name"`
	PriceFen    int64  `gorm:"column:price_fen;type:bigint;not null" json:"price_fen"`
	Quantity    int64  `gorm:"type:bigint;not null" json:"quantity"`
}

func (OrderItem) TableName() string { return "order_items_v2" }

type TimeoutOutbox struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	OrderID   int64     `gorm:"column:order_id;not null;uniqueIndex:uk_order_timeout_outbox_order" json:"order_id"`
	DueAt     time.Time `gorm:"column:due_at;not null;index" json:"due_at"`
	Status    string    `gorm:"type:varchar(20);not null;index" json:"status"`
	Attempts  int       `gorm:"not null;default:0" json:"attempts"`
	LastError string    `gorm:"column:last_error;type:varchar(500);not null;default:''" json:"last_error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (TimeoutOutbox) TableName() string { return "order_timeout_outbox_v2" }

type CreateOrderItemRequest struct {
	ProductID int64 `json:"product_id"`
	Quantity  int64 `json:"quantity"`
}

type CreateOrderRequest struct {
	IdempotencyKey string                   `json:"idempotency_key"`
	Items          []CreateOrderItemRequest `json:"items"`
}

type Service struct {
	db           *gorm.DB
	catalog      *CatalogClient
	inventory    *InventoryClient
	timeoutDelay time.Duration
}

func NewService(db *gorm.DB, catalog *CatalogClient, inventory *InventoryClient, timeoutDelay time.Duration) *Service {
	if timeoutDelay <= 0 {
		timeoutDelay = 30 * time.Minute
	}
	return &Service{db: db, catalog: catalog, inventory: inventory, timeoutDelay: timeoutDelay}
}

func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(&Order{}, &OrderItem{}, &TimeoutOutbox{})
}

func (s *Service) Create(ctx context.Context, userID int64, req CreateOrderRequest) (*Order, error) {
	ctx, span := platformtelemetry.Tracer().Start(ctx, "order.create_saga")
	span.SetAttributes(attribute.String("go_order.saga.operation", "create"))
	defer span.End()
	if userID <= 0 {
		return nil, ErrInvalidOrderRequest
	}
	key := strings.TrimSpace(req.IdempotencyKey)
	if key == "" || len(key) > 64 {
		return nil, ErrInvalidOrderRequest
	}

	if existing, err := s.findByIdempotency(ctx, userID, key); err == nil {
		return existing, nil
	} else if !errors.Is(err, ErrOrderNotFound) {
		return nil, err
	}

	items, reserveItems, total, err := s.resolveItems(ctx, req.Items)
	if err != nil {
		return nil, err
	}
	reservationID := uuid.NewString()

	order := &Order{
		UserID:         userID,
		Status:         OrderStatusReserving,
		TotalFen:       total,
		ReservationID:  reservationID,
		IdempotencyKey: key,
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(order).Error; err != nil {
			return err
		}
		for i := range items {
			items[i].OrderID = order.ID
		}
		return tx.Create(&items).Error
	})
	if err != nil {
		if existing, lookupErr := s.findByIdempotency(ctx, userID, key); lookupErr == nil {
			return existing, nil
		}
		return nil, err
	}

	reservation, err := s.inventory.Reserve(ctx, order.ID, reservationID, reserveItems)
	if err != nil {
		_ = s.db.WithContext(ctx).Model(&Order{}).Where("id = ?", order.ID).Updates(map[string]any{
			"status":         OrderStatusFailed,
			"failure_reason": truncate(err.Error(), 500),
		}).Error
		return nil, fmt.Errorf("%w: %w", ErrReservationFailed, err)
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		update := tx.Model(&Order{}).
			Where("id = ? AND status = ?", order.ID, OrderStatusReserving).
			Updates(map[string]any{
				"status":         OrderStatusPending,
				"reservation_id": reservation.ID,
				"failure_reason": "",
			})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return ErrInvalidOrderState
		}
		return tx.Create(&TimeoutOutbox{
			OrderID: order.ID,
			DueAt:   time.Now().Add(s.timeoutDelay),
			Status:  OutboxPending,
		}).Error
	})
	if err != nil {
		releaseErr := s.releaseWithTimeout(reservation.ID)
		status := OrderStatusFailed
		reason := err.Error()
		if releaseErr != nil {
			status = OrderStatusReconciliationRequired
			reason = reason + "; release compensation failed: " + releaseErr.Error()
		}
		_ = s.db.WithContext(context.Background()).Model(&Order{}).Where("id = ?", order.ID).Updates(map[string]any{
			"status":         status,
			"failure_reason": truncate(reason, 500),
		}).Error
		if releaseErr != nil {
			return nil, ErrCompensationRequired
		}
		return nil, err
	}

	return s.Get(ctx, userID, order.ID)
}

func (s *Service) Get(ctx context.Context, userID, orderID int64) (*Order, error) {
	var order Order
	if err := s.db.WithContext(ctx).Preload("Items").First(&order, "id = ? AND user_id = ?", orderID, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}
	return &order, nil
}

func (s *Service) List(ctx context.Context, userID int64, page, pageSize int) ([]Order, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	query := s.db.WithContext(ctx).Model(&Order{}).Where("user_id = ?", userID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var orders []Order
	if err := query.Preload("Items").Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&orders).Error; err != nil {
		return nil, 0, err
	}
	return orders, total, nil
}

func (s *Service) Cancel(ctx context.Context, userID, orderID int64) (*Order, error) {
	return s.cancel(ctx, userID, orderID, false)
}

func (s *Service) TimeoutCancel(ctx context.Context, orderID int64) error {
	_, err := s.cancel(ctx, 0, orderID, true)
	if errors.Is(err, ErrInvalidOrderState) || errors.Is(err, ErrOrderNotFound) {
		return nil
	}
	return err
}

func (s *Service) cancel(ctx context.Context, userID, orderID int64, system bool) (*Order, error) {
	order, err := s.loadForTransition(ctx, userID, orderID, system)
	if err != nil {
		return nil, err
	}
	if order.Status == OrderStatusCancelled {
		return order, nil
	}
	if order.Status != OrderStatusPending {
		return nil, ErrInvalidOrderState
	}

	query := s.db.WithContext(ctx).Model(&Order{}).Where("id = ? AND status = ?", orderID, OrderStatusPending)
	if !system {
		query = query.Where("user_id = ?", userID)
	}
	update := query.Update("status", OrderStatusCancelling)
	if update.Error != nil {
		return nil, update.Error
	}
	if update.RowsAffected != 1 {
		return nil, ErrInvalidOrderState
	}

	if _, err := s.inventory.Release(ctx, order.ReservationID); err != nil {
		_ = s.db.WithContext(context.Background()).Model(&Order{}).
			Where("id = ? AND status = ?", orderID, OrderStatusCancelling).
			Updates(map[string]any{"status": OrderStatusPending, "failure_reason": truncate(err.Error(), 500)}).Error
		return nil, err
	}

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Order{}).Where("id = ? AND status = ?", orderID, OrderStatusCancelling).Updates(map[string]any{
			"status":         OrderStatusCancelled,
			"failure_reason": "",
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrCompensationRequired
		}
		return tx.Model(&TimeoutOutbox{}).
			Where("order_id = ? AND status IN ?", orderID, []string{OutboxPending, OutboxPublished, OutboxFailed}).
			Update("status", OutboxCompleted).Error
	}); err != nil {
		_ = s.db.WithContext(context.Background()).Model(&Order{}).Where("id = ?", orderID).Updates(map[string]any{
			"status":         OrderStatusReconciliationRequired,
			"failure_reason": truncate(err.Error(), 500),
		}).Error
		return nil, ErrCompensationRequired
	}

	return s.loadForTransition(ctx, userID, orderID, system)
}

func (s *Service) Pay(ctx context.Context, userID, orderID int64) (*Order, error) {
	order, err := s.loadForTransition(ctx, userID, orderID, false)
	if err != nil {
		return nil, err
	}
	if order.Status == OrderStatusPaid {
		return order, nil
	}
	if order.Status != OrderStatusPending {
		return nil, ErrInvalidOrderState
	}

	result := s.db.WithContext(ctx).Model(&Order{}).
		Where("id = ? AND user_id = ? AND status = ?", orderID, userID, OrderStatusPending).
		Update("status", OrderStatusPaying)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, ErrInvalidOrderState
	}

	if _, err := s.inventory.Confirm(ctx, order.ReservationID); err != nil {
		_ = s.db.WithContext(context.Background()).Model(&Order{}).
			Where("id = ? AND status = ?", orderID, OrderStatusPaying).
			Updates(map[string]any{"status": OrderStatusPending, "failure_reason": truncate(err.Error(), 500)}).Error
		return nil, err
	}

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		update := tx.Model(&Order{}).Where("id = ? AND status = ?", orderID, OrderStatusPaying).Updates(map[string]any{
			"status":         OrderStatusPaid,
			"failure_reason": "",
		})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return ErrCompensationRequired
		}
		return tx.Model(&TimeoutOutbox{}).
			Where("order_id = ? AND status IN ?", orderID, []string{OutboxPending, OutboxPublished, OutboxFailed}).
			Update("status", OutboxCompleted).Error
	}); err != nil {
		_ = s.db.WithContext(context.Background()).Model(&Order{}).Where("id = ?", orderID).Updates(map[string]any{
			"status":         OrderStatusReconciliationRequired,
			"failure_reason": truncate(err.Error(), 500),
		}).Error
		return nil, ErrCompensationRequired
	}
	return s.Get(ctx, userID, orderID)
}

func (s *Service) Finish(ctx context.Context, userID, orderID int64) (*Order, error) {
	result := s.db.WithContext(ctx).Model(&Order{}).
		Where("id = ? AND user_id = ? AND status = ?", orderID, userID, OrderStatusPaid).
		Update("status", OrderStatusFinished)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, ErrInvalidOrderState
	}
	return s.Get(ctx, userID, orderID)
}

func (s *Service) findByIdempotency(ctx context.Context, userID int64, key string) (*Order, error) {
	var order Order
	if err := s.db.WithContext(ctx).Preload("Items").First(&order, "user_id = ? AND idempotency_key = ?", userID, key).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}
	return &order, nil
}

func (s *Service) resolveItems(ctx context.Context, requested []CreateOrderItemRequest) ([]OrderItem, []ReservationItem, int64, error) {
	if len(requested) == 0 || len(requested) > 100 {
		return nil, nil, 0, ErrInvalidOrderRequest
	}
	quantities := make(map[int64]int64, len(requested))
	for _, item := range requested {
		if item.ProductID <= 0 || item.Quantity <= 0 {
			return nil, nil, 0, ErrInvalidOrderRequest
		}
		quantities[item.ProductID] += item.Quantity
	}
	productIDs := make([]int64, 0, len(quantities))
	for productID := range quantities {
		productIDs = append(productIDs, productID)
	}
	sort.Slice(productIDs, func(i, j int) bool { return productIDs[i] < productIDs[j] })

	items := make([]OrderItem, 0, len(productIDs))
	reserveItems := make([]ReservationItem, 0, len(productIDs))
	var total int64
	for _, productID := range productIDs {
		product, err := s.catalog.GetProduct(ctx, productID)
		if err != nil {
			return nil, nil, 0, err
		}
		if product.Status != 1 || product.PriceFen <= 0 {
			return nil, nil, 0, fmt.Errorf("%w: product %d", ErrProductUnavailable, productID)
		}
		quantity := quantities[productID]
		if product.PriceFen > (1<<63-1)/quantity || total > (1<<63-1)-product.PriceFen*quantity {
			return nil, nil, 0, ErrInvalidOrderRequest
		}
		total += product.PriceFen * quantity
		items = append(items, OrderItem{
			ProductID:   product.ID,
			ProductName: product.Name,
			PriceFen:    product.PriceFen,
			Quantity:    quantity,
		})
		reserveItems = append(reserveItems, ReservationItem{ProductID: product.ID, Quantity: quantity})
	}
	return items, reserveItems, total, nil
}

func (s *Service) loadForTransition(ctx context.Context, userID, orderID int64, system bool) (*Order, error) {
	var order Order
	query := s.db.WithContext(ctx).Preload("Items").Where("id = ?", orderID)
	if !system {
		query = query.Where("user_id = ?", userID)
	}
	if err := query.First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}
	return &order, nil
}

func (s *Service) releaseWithTimeout(reservationID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.inventory.Release(ctx, reservationID)
	return err
}

type Handler struct {
	service *Service
}

func RegisterRoutes(router *gin.Engine, tokenManager *auth.TokenManager, internalToken string, service *Service) {
	h := &Handler{service: service}
	api := router.Group("/api/v1")
	api.Use(middleware.AuthMiddleware(tokenManager))
	api.POST("/orders", h.create)
	api.GET("/orders/:id", h.get)
	api.GET("/orders", h.list)
	api.PATCH("/orders/:id/cancel", h.cancel)
	api.PATCH("/orders/:id/pay", h.pay)
	api.PATCH("/orders/:id/finish", h.finish)

	internal := router.Group("/internal/v1")
	internal.Use(internalapi.Middleware(internalToken))
	internal.POST("/orders/:id/timeout-cancel", h.timeoutCancel)
}

func (h *Handler) create(c *gin.Context) {
	userID, okUser := currentUserID(c)
	if !okUser {
		fail(c, http.StatusUnauthorized, 40121, "invalid authenticated user")
		return
	}
	var req CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, 40021, "invalid order request")
		return
	}
	order, err := h.service.Create(c.Request.Context(), userID, req)
	if err != nil {
		writeOrderError(c, err)
		return
	}
	ok(c, http.StatusCreated, order)
}

func (h *Handler) get(c *gin.Context) {
	userID, orderID, valid := requestOrderIDs(c)
	if !valid {
		fail(c, http.StatusBadRequest, 40022, "invalid order id")
		return
	}
	order, err := h.service.Get(c.Request.Context(), userID, orderID)
	if err != nil {
		writeOrderError(c, err)
		return
	}
	ok(c, http.StatusOK, order)
}

func (h *Handler) list(c *gin.Context) {
	userID, okUser := currentUserID(c)
	if !okUser {
		fail(c, http.StatusUnauthorized, 40121, "invalid authenticated user")
		return
	}
	page := parsePositiveInt(c.Query("page"), 1)
	pageSize := parsePositiveInt(c.Query("page_size"), 20)
	orders, total, err := h.service.List(c.Request.Context(), userID, page, pageSize)
	if err != nil {
		writeOrderError(c, err)
		return
	}
	ok(c, http.StatusOK, gin.H{"list": orders, "total": total, "page": page, "page_size": pageSize})
}

func (h *Handler) cancel(c *gin.Context) { h.transition(c, h.service.Cancel) }
func (h *Handler) pay(c *gin.Context)    { h.transition(c, h.service.Pay) }
func (h *Handler) finish(c *gin.Context) { h.transition(c, h.service.Finish) }

func (h *Handler) transition(c *gin.Context, action func(context.Context, int64, int64) (*Order, error)) {
	userID, orderID, valid := requestOrderIDs(c)
	if !valid {
		fail(c, http.StatusBadRequest, 40022, "invalid order id")
		return
	}
	order, err := action(c.Request.Context(), userID, orderID)
	if err != nil {
		writeOrderError(c, err)
		return
	}
	ok(c, http.StatusOK, order)
}

func (h *Handler) timeoutCancel(c *gin.Context) {
	orderID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || orderID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid order id"})
		return
	}
	if err := h.service.TimeoutCancel(c.Request.Context(), orderID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "processed"})
}

func currentUserID(c *gin.Context) (int64, bool) {
	value, exists := c.Get(middleware.UserIDKey)
	userID, ok := value.(int64)
	return userID, exists && ok && userID > 0
}

func requestOrderIDs(c *gin.Context) (int64, int64, bool) {
	userID, okUser := currentUserID(c)
	orderID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	return userID, orderID, okUser && err == nil && orderID > 0
}

func parsePositiveInt(raw string, fallback int) int {
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func writeOrderError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrOrderNotFound):
		fail(c, http.StatusNotFound, 40421, "order not found")
	case errors.Is(err, ErrInvalidOrderRequest):
		fail(c, http.StatusBadRequest, 40021, "invalid order request")
	case errors.Is(err, ErrInvalidOrderState):
		fail(c, http.StatusConflict, 40921, "invalid order state")
	case errors.Is(err, ErrProductUnavailable), errors.Is(err, ErrReservationFailed):
		fail(c, http.StatusConflict, 40922, err.Error())
	case errors.Is(err, ErrCompensationRequired):
		fail(c, http.StatusServiceUnavailable, 50321, "order requires reconciliation")
	default:
		var remote *RemoteError
		if errors.As(err, &remote) {
			fail(c, http.StatusBadGateway, 50221, remote.Error())
			return
		}
		fail(c, http.StatusInternalServerError, 50021, "order operation failed")
	}
}

func ok(c *gin.Context, status int, data any) {
	c.JSON(status, gin.H{"code": 0, "msg": "success", "data": data})
}

func fail(c *gin.Context, status, code int, message string) {
	c.JSON(status, gin.H{"code": code, "msg": message})
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
