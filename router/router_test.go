package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go-order-management-system/internal/auth"
	"go-order-management-system/internal/handler"

	"github.com/gin-gonic/gin"
)

type denyRoleChecker struct {
	calls int
}

func (c *denyRoleChecker) HasRole(context.Context, int64, string) (bool, error) {
	c.calls++
	return false, nil
}

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
	}, tokenManager, nil)

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

func TestAdminRoutesRequireAdminRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tokenManager, err := auth.NewTokenManager("0123456789abcdef0123456789abcdef", "test", time.Hour)
	if err != nil {
		t.Fatalf("new token manager: %v", err)
	}
	token, err := tokenManager.GenerateAccessToken(7, "normal-user")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	checker := &denyRoleChecker{}
	r := SetupRouters(nil, Handlers{
		Product:   &handler.ProductHandler{},
		Inventory: &handler.InventoryHandler{},
		StockLog:  &handler.StockLogHandler{},
		Order:     &handler.OrderHandler{},
		Health:    &handler.HealthHandler{},
		User:      &handler.UserHandler{},
	}, tokenManager, checker)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/products", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	r.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, recorder.Code)
	}
	if checker.calls != 1 {
		t.Fatalf("expected one role check, got %d", checker.calls)
	}
}
