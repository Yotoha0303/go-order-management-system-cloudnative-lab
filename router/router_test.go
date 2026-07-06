package router

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go-order-management-system/internal/assistant"
	"go-order-management-system/internal/auth"
	"go-order-management-system/internal/handler"
	"go-order-management-system/internal/response"

	"github.com/gin-gonic/gin"
)

type denyRoleChecker struct {
	calls int
}

type allowRoleChecker struct {
	calls int
}

func (c *allowRoleChecker) HasRole(context.Context, int64, string) (bool, error) {
	c.calls++
	return true, nil
}

type assistantServiceStub struct {
	calls int
	input assistant.ChatInput
}

func (s *assistantServiceStub) Chat(_ context.Context, input assistant.ChatInput) (assistant.ChatResponse, error) {
	s.calls++
	s.input = input
	return assistant.ChatResponse{
		RequestID: input.RequestID,
		Intent:    assistant.IntentGetLowStockProducts,
		Answer:    "ok",
		Data:      map[string]int{"count": 0},
	}, nil
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
		Assistant: &handler.AssistantHandler{},
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
		Assistant: &handler.AssistantHandler{},
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

func TestAssistantRouteRequiresAdminRole(t *testing.T) {
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
		Assistant: &handler.AssistantHandler{},
	}, tokenManager, checker)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/assistant/chat", nil)
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

func TestAssistantRouteUsesJWTAdminAndAuthenticatedIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tokenManager, err := auth.NewTokenManager("0123456789abcdef0123456789abcdef", "test", time.Hour)
	if err != nil {
		t.Fatalf("new token manager: %v", err)
	}
	token, err := tokenManager.GenerateAccessToken(7, "admin-user")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	checker := &allowRoleChecker{}
	service := &assistantServiceStub{}
	assistantHandler, err := handler.NewAssistantHandler(service)
	if err != nil {
		t.Fatalf("new assistant handler: %v", err)
	}
	r := SetupRouters(nil, Handlers{
		Product:   &handler.ProductHandler{},
		Inventory: &handler.InventoryHandler{},
		StockLog:  &handler.StockLogHandler{},
		Order:     &handler.OrderHandler{},
		Health:    &handler.HealthHandler{},
		User:      &handler.UserHandler{},
		Assistant: assistantHandler,
	}, tokenManager, checker)

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/admin/assistant/chat",
		bytes.NewBufferString(`{"message":"查询低库存"}`),
	)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	r.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if checker.calls != 1 || service.calls != 1 || service.input.UserID != 7 || service.input.RequestID == "" {
		t.Fatalf("role calls=%d service calls=%d input=%+v", checker.calls, service.calls, service.input)
	}
	var body response.Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != response.CodeSuccess {
		t.Fatalf("unexpected response: %+v", body)
	}
}
