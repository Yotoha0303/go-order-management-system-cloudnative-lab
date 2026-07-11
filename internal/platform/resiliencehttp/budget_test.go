package resiliencehttp

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBudgetHandlerPropagatesDeadlineAndRequestID(t *testing.T) {
	var capturedDeadline string
	var capturedRequestID string
	next := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		capturedDeadline = req.Header.Get(DeadlineHeader)
		capturedRequestID = RequestID(req.Context())
		if _, ok := req.Context().Deadline(); !ok {
			t.Fatal("expected request context deadline")
		}
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(RequestIDHeader, "request-123")
	response := httptest.NewRecorder()

	BudgetHandler(next, BudgetConfig{Default: time.Second, Maximum: 2 * time.Second}).ServeHTTP(response, req)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", response.Code)
	}
	if capturedRequestID != "request-123" {
		t.Fatalf("expected propagated request id, got %q", capturedRequestID)
	}
	if response.Header().Get(RequestIDHeader) != "request-123" {
		t.Fatalf("expected response request id, got %q", response.Header().Get(RequestIDHeader))
	}
	if capturedDeadline == "" {
		t.Fatal("expected propagated request deadline header")
	}
	if _, err := time.Parse(time.RFC3339Nano, capturedDeadline); err != nil {
		t.Fatalf("parse propagated deadline: %v", err)
	}
}

func TestBudgetHandlerRejectsExpiredDeadline(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	})
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(DeadlineHeader, time.Now().Add(-time.Second).UTC().Format(time.RFC3339Nano))
	response := httptest.NewRecorder()

	BudgetHandler(next, BudgetConfig{Default: time.Second, Maximum: 2 * time.Second}).ServeHTTP(response, req)

	if called {
		t.Fatal("expired request reached downstream handler")
	}
	if response.Code != http.StatusGatewayTimeout {
		t.Fatalf("expected 504, got %d", response.Code)
	}
}

func TestApplyMetadataUsesBudgetContext(t *testing.T) {
	var capturedRequest *http.Request
	next := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		outbound, err := http.NewRequestWithContext(req.Context(), http.MethodGet, "http://service.local", nil)
		if err != nil {
			t.Fatalf("build outbound request: %v", err)
		}
		ApplyMetadata(req.Context(), outbound)
		capturedRequest = outbound
		w.WriteHeader(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(RequestIDHeader, "request-456")
	response := httptest.NewRecorder()

	BudgetHandler(next, BudgetConfig{Default: time.Second, Maximum: 2 * time.Second}).ServeHTTP(response, req)

	if capturedRequest == nil {
		t.Fatal("outbound request was not captured")
	}
	if capturedRequest.Header.Get(RequestIDHeader) != "request-456" {
		t.Fatalf("expected outbound request id, got %q", capturedRequest.Header.Get(RequestIDHeader))
	}
	if capturedRequest.Header.Get(DeadlineHeader) == "" {
		t.Fatal("expected outbound deadline header")
	}
}
