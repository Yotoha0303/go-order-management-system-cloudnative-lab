package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go-order-management-system/internal/auth"
	"go-order-management-system/internal/handler"

	"github.com/gin-gonic/gin"
)

func TestBusinessRoutesRequireAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tokenManager, err := auth.NewTokenManager("0123456789abcdef0123456789abcdef", "test", time.Hour)
	if err != nil {
		t.Fatalf("new token manager: %v", err)
	}
	router := SetupRouters(nil, Handlers{
		Product:   &handler.ProductHandler{},
		Inventory: &handler.InventoryHandler{},
		StockLog:  &handler.StockLogHandler{},
		Order:     &handler.OrderHandler{},
		Health:    &handler.HealthHandler{},
		User:      &handler.UserHandler{},
	}, tokenManager)

	for _, path := range []string{
		"/api/v1/products",
		"/api/v1/inventory/products/1",
		"/api/v1/stock-logs",
		"/api/v1/orders",
		"/api/v1/users/me",
	} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("expected %s to require authentication, got %d", path, recorder.Code)
		}
	}
}
