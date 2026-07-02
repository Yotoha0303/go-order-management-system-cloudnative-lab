package handler

import (
	"context"
	"go-order-management-system/internal/response"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type databasePinger interface {
	PingContext(ctx context.Context) error
}

type HealthHandler struct {
	db databasePinger
}

func NewHealthHandler(db *gorm.DB) *HealthHandler {
	if db == nil {
		return &HealthHandler{}
	}

	sqlDB, err := db.DB()
	if err != nil {
		return &HealthHandler{}
	}

	return &HealthHandler{
		db: sqlDB,
	}
}

func (h *HealthHandler) PingHandler(c *gin.Context) {
	response.Success(c, gin.H{
		"message": "success",
	})
}

func (h *HealthHandler) LiveHandler(c *gin.Context) {
	response.Success(c, gin.H{
		"message": "live",
	})
}

func (h *HealthHandler) ReadyzHandler(c *gin.Context) {
	if h == nil || h.db == nil {
		response.Fail(c, http.StatusServiceUnavailable, response.CodeReadinessFailed, "database is not initialized")
		return
	}

	if err := h.db.PingContext(c.Request.Context()); err != nil {
		response.Fail(c, http.StatusServiceUnavailable, response.CodeReadinessFailed, "database is not ready")
		return
	}

	response.Success(c, gin.H{
		"status": "ready",
	})
}
