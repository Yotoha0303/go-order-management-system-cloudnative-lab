package inventorysvc

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

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ReservationPending   = "pending"
	ReservationConfirmed = "confirmed"
	ReservationReleased  = "released"
)

var (
	ErrInventoryNotFound      = errors.New("inventory not found")
	ErrInsufficientInventory  = errors.New("insufficient inventory")
	ErrReservationNotFound    = errors.New("reservation not found")
	ErrReservationTransition  = errors.New("invalid reservation transition")
	ErrInvalidInventoryAmount = errors.New("invalid inventory amount")
)

type Inventory struct {
	ProductID         int64     `gorm:"primaryKey;column:product_id" json:"product_id"`
	AvailableQuantity int64     `gorm:"column:available_quantity;type:bigint;not null" json:"available_quantity"`
	ReservedQuantity  int64     `gorm:"column:reserved_quantity;type:bigint;not null;default:0" json:"reserved_quantity"`
	Version           int64     `gorm:"type:bigint;not null;default:0" json:"version"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func (Inventory) TableName() string { return "inventory_items" }

type Reservation struct {
	ID        string            `gorm:"primaryKey;type:varchar(36)" json:"id"`
	OrderID   int64             `gorm:"column:order_id;not null;uniqueIndex:uk_inventory_reservation_order" json:"order_id"`
	Status    string            `gorm:"type:varchar(20);not null;index" json:"status"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
	Items     []ReservationItem `gorm:"foreignKey:ReservationID" json:"items,omitempty"`
}

func (Reservation) TableName() string { return "inventory_reservations" }

type ReservationItem struct {
	ID            uint64 `gorm:"primaryKey;autoIncrement" json:"id"`
	ReservationID string `gorm:"type:varchar(36);not null;uniqueIndex:uk_reservation_product,priority:1;index" json:"reservation_id"`
	ProductID     int64  `gorm:"not null;uniqueIndex:uk_reservation_product,priority:2" json:"product_id"`
	Quantity      int64  `gorm:"type:bigint;not null" json:"quantity"`
}

func (ReservationItem) TableName() string { return "inventory_reservation_items" }

type StockLog struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	ProductID   int64     `gorm:"not null;index" json:"product_id"`
	ChangeType  string    `gorm:"type:varchar(32);not null;index" json:"change_type"`
	Quantity    int64     `gorm:"type:bigint;not null" json:"quantity"`
	ReferenceID string    `gorm:"type:varchar(64);not null;default:'';index" json:"reference_id"`
	CreatedAt   time.Time `json:"created_at"`
}

func (StockLog) TableName() string { return "inventory_stock_logs" }

type ItemRequest struct {
	ProductID int64 `json:"product_id"`
	Quantity  int64 `json:"quantity"`
}

type ReserveRequest struct {
	OrderID       int64         `json:"order_id"`
	ReservationID string        `json:"reservation_id"`
	Items         []ItemRequest `json:"items"`
}

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service { return &Service{db: db} }

func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(&Inventory{}, &Reservation{}, &ReservationItem{}, &StockLog{})
}

func (s *Service) Get(ctx context.Context, productID int64) (*Inventory, error) {
	var inventory Inventory
	if err := s.db.WithContext(ctx).First(&inventory, "product_id = ?", productID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInventoryNotFound
		}
		return nil, err
	}
	return &inventory, nil
}

func (s *Service) Init(ctx context.Context, productID, quantity int64) (*Inventory, error) {
	if productID <= 0 || quantity < 0 {
		return nil, ErrInvalidInventoryAmount
	}
	inventory := &Inventory{ProductID: productID, AvailableQuantity: quantity}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(inventory).Error; err != nil {
			return err
		}
		return tx.Create(&StockLog{
			ProductID:  productID,
			ChangeType: "initialize",
			Quantity:   quantity,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return inventory, nil
}

func (s *Service) Add(ctx context.Context, productID, quantity int64) (*Inventory, error) {
	if productID <= 0 || quantity <= 0 {
		return nil, ErrInvalidInventoryAmount
	}
	var inventory Inventory
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&inventory, "product_id = ?", productID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInventoryNotFound
			}
			return err
		}
		if err := tx.Model(&Inventory{}).Where("product_id = ?", productID).Updates(map[string]any{
			"available_quantity": gorm.Expr("available_quantity + ?", quantity),
			"version":            gorm.Expr("version + 1"),
		}).Error; err != nil {
			return err
		}
		if err := tx.Create(&StockLog{ProductID: productID, ChangeType: "add", Quantity: quantity}).Error; err != nil {
			return err
		}
		return tx.First(&inventory, "product_id = ?", productID).Error
	})
	if err != nil {
		return nil, err
	}
	return &inventory, nil
}

func (s *Service) Reserve(ctx context.Context, req ReserveRequest) (*Reservation, error) {
	items, err := normalizeItems(req.Items)
	if err != nil || req.OrderID <= 0 {
		return nil, ErrInvalidInventoryAmount
	}
	reservationID := strings.TrimSpace(req.ReservationID)
	if reservationID == "" {
		reservationID = uuid.NewString()
	}

	var result Reservation
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing Reservation
		if err := tx.Preload("Items").First(&existing, "order_id = ?", req.OrderID).Error; err == nil {
			result = existing
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		reservation := Reservation{ID: reservationID, OrderID: req.OrderID, Status: ReservationPending}
		if err := tx.Create(&reservation).Error; err != nil {
			return err
		}

		for _, item := range items {
			var inventory Inventory
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&inventory, "product_id = ?", item.ProductID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return fmt.Errorf("%w: product %d", ErrInventoryNotFound, item.ProductID)
				}
				return err
			}
			if inventory.AvailableQuantity < item.Quantity {
				return fmt.Errorf("%w: product %d", ErrInsufficientInventory, item.ProductID)
			}

			update := tx.Model(&Inventory{}).
				Where("product_id = ? AND available_quantity >= ?", item.ProductID, item.Quantity).
				Updates(map[string]any{
					"available_quantity": gorm.Expr("available_quantity - ?", item.Quantity),
					"reserved_quantity":  gorm.Expr("reserved_quantity + ?", item.Quantity),
					"version":            gorm.Expr("version + 1"),
				})
			if update.Error != nil {
				return update.Error
			}
			if update.RowsAffected != 1 {
				return fmt.Errorf("%w: product %d", ErrInsufficientInventory, item.ProductID)
			}
			if err := tx.Create(&ReservationItem{ReservationID: reservationID, ProductID: item.ProductID, Quantity: item.Quantity}).Error; err != nil {
				return err
			}
			if err := tx.Create(&StockLog{ProductID: item.ProductID, ChangeType: "reserve", Quantity: -item.Quantity, ReferenceID: reservationID}).Error; err != nil {
				return err
			}
		}

		return tx.Preload("Items").First(&result, "id = ?", reservationID).Error
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *Service) Confirm(ctx context.Context, reservationID string) (*Reservation, error) {
	return s.transition(ctx, reservationID, ReservationConfirmed)
}

func (s *Service) Release(ctx context.Context, reservationID string) (*Reservation, error) {
	return s.transition(ctx, reservationID, ReservationReleased)
}

func (s *Service) transition(ctx context.Context, reservationID, target string) (*Reservation, error) {
	reservationID = strings.TrimSpace(reservationID)
	if reservationID == "" {
		return nil, ErrReservationNotFound
	}
	var result Reservation
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var reservation Reservation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Preload("Items").First(&reservation, "id = ?", reservationID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrReservationNotFound
			}
			return err
		}
		if reservation.Status == target {
			result = reservation
			return nil
		}
		if reservation.Status != ReservationPending {
			return ErrReservationTransition
		}

		items := append([]ReservationItem(nil), reservation.Items...)
		sort.Slice(items, func(i, j int) bool { return items[i].ProductID < items[j].ProductID })
		for _, item := range items {
			var inventory Inventory
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&inventory, "product_id = ?", item.ProductID).Error; err != nil {
				return err
			}
			updates := map[string]any{
				"reserved_quantity": gorm.Expr("reserved_quantity - ?", item.Quantity),
				"version":           gorm.Expr("version + 1"),
			}
			changeType := "confirm"
			logQuantity := -item.Quantity
			if target == ReservationReleased {
				updates["available_quantity"] = gorm.Expr("available_quantity + ?", item.Quantity)
				changeType = "release"
				logQuantity = item.Quantity
			}
			update := tx.Model(&Inventory{}).
				Where("product_id = ? AND reserved_quantity >= ?", item.ProductID, item.Quantity).
				Updates(updates)
			if update.Error != nil {
				return update.Error
			}
			if update.RowsAffected != 1 {
				return ErrReservationTransition
			}
			if err := tx.Create(&StockLog{ProductID: item.ProductID, ChangeType: changeType, Quantity: logQuantity, ReferenceID: reservationID}).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&Reservation{}).Where("id = ?", reservationID).Update("status", target).Error; err != nil {
			return err
		}
		reservation.Status = target
		result = reservation
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *Service) ListLogs(ctx context.Context, page, pageSize int) ([]StockLog, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	var total int64
	if err := s.db.WithContext(ctx).Model(&StockLog{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var logs []StockLog
	if err := s.db.WithContext(ctx).Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&logs).Error; err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}

func normalizeItems(items []ItemRequest) ([]ItemRequest, error) {
	if len(items) == 0 || len(items) > 100 {
		return nil, ErrInvalidInventoryAmount
	}
	quantities := make(map[int64]int64, len(items))
	for _, item := range items {
		if item.ProductID <= 0 || item.Quantity <= 0 {
			return nil, ErrInvalidInventoryAmount
		}
		quantities[item.ProductID] += item.Quantity
	}
	result := make([]ItemRequest, 0, len(quantities))
	for productID, quantity := range quantities {
		result = append(result, ItemRequest{ProductID: productID, Quantity: quantity})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ProductID < result[j].ProductID })
	return result, nil
}

type Handler struct {
	service *Service
}

func RegisterRoutes(router *gin.Engine, tokenManager *auth.TokenManager, roleChecker middleware.RoleChecker, internalToken string, service *Service) {
	h := &Handler{service: service}
	api := router.Group("/api/v1")
	api.Use(middleware.AuthMiddleware(tokenManager))
	api.GET("/inventory/products/:product_id", h.get)

	admin := api.Group("")
	admin.Use(middleware.AdminMiddleware(roleChecker))
	admin.POST("/inventory/init", h.init)
	admin.POST("/inventory/add", h.add)
	admin.GET("/stock-logs", h.listLogs)

	internal := router.Group("/internal/v1")
	internal.Use(internalapi.Middleware(internalToken))
	internal.POST("/reservations", h.reserve)
	internal.POST("/reservations/:id/confirm", h.confirm)
	internal.POST("/reservations/:id/release", h.release)
}

func (h *Handler) get(c *gin.Context) {
	productID, okID := parseID(c.Param("product_id"))
	if !okID {
		fail(c, http.StatusBadRequest, 40011, "invalid product id")
		return
	}
	inventory, err := h.service.Get(c.Request.Context(), productID)
	if err != nil {
		if errors.Is(err, ErrInventoryNotFound) {
			fail(c, http.StatusNotFound, 40411, "inventory not found")
			return
		}
		fail(c, http.StatusInternalServerError, 50011, "get inventory failed")
		return
	}
	ok(c, http.StatusOK, inventoryPayload(inventory))
}

func (h *Handler) init(c *gin.Context) { h.change(c, true) }
func (h *Handler) add(c *gin.Context)  { h.change(c, false) }

func (h *Handler) change(c *gin.Context, initialize bool) {
	var req struct {
		ProductID     int64 `json:"product_id"`
		Quantity      int64 `json:"quantity"`
		StockQuantity int64 `json:"stock_quantity"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, 40012, "invalid request")
		return
	}
	quantity := req.Quantity
	if quantity == 0 {
		quantity = req.StockQuantity
	}
	var inventory *Inventory
	var err error
	if initialize {
		inventory, err = h.service.Init(c.Request.Context(), req.ProductID, quantity)
	} else {
		inventory, err = h.service.Add(c.Request.Context(), req.ProductID, quantity)
	}
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, ErrInventoryNotFound) {
			status = http.StatusNotFound
		}
		fail(c, status, 40013, err.Error())
		return
	}
	ok(c, http.StatusOK, inventoryPayload(inventory))
}

func (h *Handler) listLogs(c *gin.Context) {
	page := parsePositiveInt(c.Query("page"), 1)
	pageSize := parsePositiveInt(c.Query("page_size"), 20)
	logs, total, err := h.service.ListLogs(c.Request.Context(), page, pageSize)
	if err != nil {
		fail(c, http.StatusInternalServerError, 50012, "list stock logs failed")
		return
	}
	ok(c, http.StatusOK, gin.H{"list": logs, "total": total, "page": page, "page_size": pageSize})
}

func (h *Handler) reserve(c *gin.Context) {
	var req ReserveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	reservation, err := h.service.Reserve(c.Request.Context(), req)
	if err != nil {
		status := http.StatusConflict
		if !errors.Is(err, ErrInsufficientInventory) && !errors.Is(err, ErrInventoryNotFound) && !errors.Is(err, ErrInvalidInventoryAmount) {
			status = http.StatusInternalServerError
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, reservation)
}

func (h *Handler) confirm(c *gin.Context) { h.transition(c, true) }
func (h *Handler) release(c *gin.Context) { h.transition(c, false) }

func (h *Handler) transition(c *gin.Context, confirm bool) {
	var reservation *Reservation
	var err error
	if confirm {
		reservation, err = h.service.Confirm(c.Request.Context(), c.Param("id"))
	} else {
		reservation, err = h.service.Release(c.Request.Context(), c.Param("id"))
	}
	if err != nil {
		status := http.StatusConflict
		if errors.Is(err, ErrReservationNotFound) {
			status = http.StatusNotFound
		} else if !errors.Is(err, ErrReservationTransition) {
			status = http.StatusInternalServerError
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, reservation)
}

func inventoryPayload(inventory *Inventory) gin.H {
	return gin.H{
		"product_id":         inventory.ProductID,
		"stock_quantity":     inventory.AvailableQuantity,
		"available_quantity": inventory.AvailableQuantity,
		"reserved_quantity":  inventory.ReservedQuantity,
		"version":            inventory.Version,
		"created_at":         inventory.CreatedAt,
		"updated_at":         inventory.UpdatedAt,
	}
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
