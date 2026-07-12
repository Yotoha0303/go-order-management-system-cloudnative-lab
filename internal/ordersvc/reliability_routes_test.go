package ordersvc

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go-order-management-system/internal/platform/internalapi"

	"github.com/gin-gonic/gin"
)

type stubReliabilitySnapshotter struct {
	snapshot ReliabilitySnapshot
	err      error
	calls    int
}

func (stub *stubReliabilitySnapshotter) Snapshot(context.Context) (ReliabilitySnapshot, error) {
	stub.calls++
	return stub.snapshot, stub.err
}

func TestReliabilityEndpointRequiresInternalToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &stubReliabilitySnapshotter{}
	router := gin.New()
	RegisterReliabilityRoutes(router, "internal-token", stub)

	req := httptest.NewRequest(http.MethodGet, "/internal/v1/operations/reliability", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", recorder.Code)
	}
	if stub.calls != 0 {
		t.Fatalf("unauthorized request reached snapshotter: calls=%d", stub.calls)
	}
}

func TestReliabilityEndpointReturnsStableSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	collectedAt := time.Date(2026, time.July, 11, 12, 0, 0, 0, time.UTC)
	stub := &stubReliabilitySnapshotter{snapshot: ReliabilitySnapshot{
		CollectedAt:     collectedAt,
		QueryDurationMS: 4,
		Outbox: OutboxIndicators{
			ByStatus:   OutboxStatusIndicators{Pending: 2, Failed: 1},
			RetryReady: 3,
		},
		Orders: OrderSagaIndicators{
			ReconciliationRequired: 1,
			StuckTransient:         2,
		},
	}}
	router := gin.New()
	RegisterReliabilityRoutes(router, "internal-token", stub)

	req := httptest.NewRequest(http.MethodGet, "/internal/v1/operations/reliability", nil)
	req.Header.Set(internalapi.Header, "internal-token")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var response ReliabilitySnapshot
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if response.CollectedAt != collectedAt || response.QueryDurationMS != 4 {
		t.Fatalf("unexpected snapshot metadata: %+v", response)
	}
	if response.Outbox.ByStatus.Pending != 2 || response.Outbox.ByStatus.Failed != 1 || response.Outbox.RetryReady != 3 {
		t.Fatalf("unexpected outbox response: %+v", response.Outbox)
	}
	if response.Orders.ReconciliationRequired != 1 || response.Orders.StuckTransient != 2 {
		t.Fatalf("unexpected order response: %+v", response.Orders)
	}
	if stub.calls != 1 {
		t.Fatalf("expected one snapshot call, got %d", stub.calls)
	}
}

func TestReliabilityEndpointHidesCollectionError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &stubReliabilitySnapshotter{err: errors.New("database details must not leak")}
	router := gin.New()
	RegisterReliabilityRoutes(router, "internal-token", stub)

	req := httptest.NewRequest(http.MethodGet, "/internal/v1/operations/reliability", nil)
	req.Header.Set(internalapi.Header, "internal-token")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", recorder.Code)
	}
	body := recorder.Body.String()
	if body == "" || strings.Contains(body, "database details") {
		t.Fatalf("endpoint leaked collection error: %s", body)
	}
}
