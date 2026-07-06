package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go-order-management-system/internal/middleware"
	"go-order-management-system/internal/model"
	"go-order-management-system/internal/request"
	"go-order-management-system/internal/response"
	"go-order-management-system/internal/service"

	"github.com/gin-gonic/gin"
)

type stubOrderService struct {
	createResult *model.Order
	createErr    error
	createCalls  int
	userID       int64
	createReq    request.CreateOrderRequest
}

func (s *stubOrderService) CreateOrder(_ context.Context, userID int64, req request.CreateOrderRequest) (*model.Order, error) {
	s.createCalls++
	s.userID = userID
	s.createReq = req
	return s.createResult, s.createErr
}

func (*stubOrderService) ListOrders(context.Context, int64, int, int) ([]*model.Order, int64, error) {
	panic("unexpected ListOrders call")
}

func (*stubOrderService) GetOrderByID(context.Context, int64, int64) (*model.Order, []*model.OrderItem, error) {
	panic("unexpected GetOrderByID call")
}

func (*stubOrderService) PayOrder(context.Context, int64, int64) error {
	panic("unexpected PayOrder call")
}

func (*stubOrderService) FinishOrder(context.Context, int64, int64) error {
	panic("unexpected FinishOrder call")
}

func (*stubOrderService) CancelOrder(context.Context, int64, int64) error {
	panic("unexpected CancelOrder call")
}

func TestOrderHandlerCreateOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("binds identity and request then returns order", func(t *testing.T) {
		stub := &stubOrderService{createResult: &model.Order{ID: 99, OrderNo: "ORDER-99"}}
		recorder := performCreateOrderRequest(t, stub, `{
			"idempotency_key":"request-1",
			"items":[{"product_id":7,"quantity":2}]
		}`)

		if recorder.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", recorder.Code)
		}
		if stub.createCalls != 1 || stub.userID != 42 {
			t.Fatalf("unexpected service call: calls=%d userID=%d", stub.createCalls, stub.userID)
		}
		if stub.createReq.IdempotencyKey != "request-1" || len(stub.createReq.Items) != 1 || stub.createReq.Items[0].ProductID != 7 || stub.createReq.Items[0].Quantity != 2 {
			t.Fatalf("unexpected create request: %+v", stub.createReq)
		}

		var body struct {
			Code int `json:"code"`
			Data struct {
				ID      int64  `json:"id"`
				OrderNo string `json:"order_no"`
			} `json:"data"`
		}
		decodeHandlerResponse(t, recorder, &body)
		if body.Code != response.CodeSuccess || body.Data.ID != 99 || body.Data.OrderNo != "ORDER-99" {
			t.Fatalf("unexpected response: %+v", body)
		}
	})

	t.Run("rejects invalid body before calling service", func(t *testing.T) {
		stub := &stubOrderService{}
		recorder := performCreateOrderRequest(t, stub, `{
			"idempotency_key":"request-2",
			"items":[{"product_id":7,"quantity":0}]
		}`)

		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", recorder.Code)
		}
		if stub.createCalls != 0 {
			t.Fatalf("expected service not to be called, got %d calls", stub.createCalls)
		}
	})

	t.Run("maps business error to HTTP response", func(t *testing.T) {
		stub := &stubOrderService{createErr: service.ErrInsufficientStock}
		recorder := performCreateOrderRequest(t, stub, `{
			"idempotency_key":"request-3",
			"items":[{"product_id":7,"quantity":2}]
		}`)

		if recorder.Code != http.StatusConflict {
			t.Fatalf("expected status 409, got %d", recorder.Code)
		}
		var body struct {
			Code int `json:"code"`
		}
		decodeHandlerResponse(t, recorder, &body)
		if body.Code != response.CodeInsufficientStock {
			t.Fatalf("expected code %d, got %d", response.CodeInsufficientStock, body.Code)
		}
	})
}

func performCreateOrderRequest(t *testing.T, service OrderService, payload string) *httptest.ResponseRecorder {
	t.Helper()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.UserIDKey, int64(42))
		c.Next()
	})
	router.POST("/orders", NewOrderHandler(service).CreateOrder)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)
	return recorder
}

func decodeHandlerResponse(t *testing.T, recorder *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(recorder.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response failed: %v; body=%s", err, recorder.Body.String())
	}
}
