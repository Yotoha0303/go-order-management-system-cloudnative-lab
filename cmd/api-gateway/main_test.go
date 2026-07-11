package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go-order-management-system/internal/platform/ratelimit"
	"go-order-management-system/internal/platform/resiliencehttp"
)

func TestGatewayReturnsRateLimitContract(t *testing.T) {
	handler := resiliencehttp.BudgetHandler(&gateway{
		limiter: ratelimit.New(ratelimit.Config{
			PerClientRate:  0.001,
			PerClientBurst: 1,
			GlobalRate:     100,
			GlobalBurst:    100,
		}),
	}, resiliencehttp.BudgetConfig{Default: time.Second, Maximum: time.Second})

	first := httptest.NewRequest(http.MethodGet, "/missing", nil)
	first.RemoteAddr = "203.0.113.10:41000"
	firstRecorder := httptest.NewRecorder()
	handler.ServeHTTP(firstRecorder, first)
	if firstRecorder.Code != http.StatusNotFound {
		t.Fatalf("expected first request to reach routing, got %d", firstRecorder.Code)
	}

	second := httptest.NewRequest(http.MethodGet, "/missing", nil)
	second.RemoteAddr = "203.0.113.10:41001"
	secondRecorder := httptest.NewRecorder()
	handler.ServeHTTP(secondRecorder, second)
	if secondRecorder.Code != http.StatusTooManyRequests {
		t.Fatalf("expected HTTP 429, got %d", secondRecorder.Code)
	}
	if secondRecorder.Header().Get("Retry-After") == "" {
		t.Fatal("rate-limit response is missing Retry-After")
	}
	if secondRecorder.Header().Get(resiliencehttp.RequestIDHeader) == "" {
		t.Fatal("rate-limit response is missing X-Request-ID")
	}

	var payload struct {
		Code       string `json:"code"`
		RequestID  string `json:"request_id"`
		RetryAfter int64  `json:"retry_after"`
	}
	if err := json.Unmarshal(secondRecorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode rate-limit response: %v", err)
	}
	if payload.Code != "rate_limited" || payload.RequestID == "" || payload.RetryAfter < 1 {
		t.Fatalf("unexpected rate-limit payload: %+v", payload)
	}
}

func TestGatewayHealthEndpointsBypassRateLimit(t *testing.T) {
	handler := resiliencehttp.BudgetHandler(&gateway{
		limiter: ratelimit.New(ratelimit.Config{
			PerClientRate:  0.001,
			PerClientBurst: 1,
			GlobalRate:     0.001,
			GlobalBurst:    1,
		}),
	}, resiliencehttp.BudgetConfig{Default: time.Second, Maximum: time.Second})

	consume := httptest.NewRequest(http.MethodGet, "/missing", nil)
	consume.RemoteAddr = "198.51.100.20:42000"
	handler.ServeHTTP(httptest.NewRecorder(), consume)

	for _, path := range []string{"/live", "/readyz"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.RemoteAddr = "198.51.100.20:42001"
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusOK {
			t.Fatalf("health endpoint %s was rate limited: status=%d", path, recorder.Code)
		}
	}
}

func TestClientKeyUsesRemoteIPWithoutPort(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "[2001:db8::1]:443"
	if key := clientKey(req); key != "2001:db8::1" {
		t.Fatalf("unexpected client key %q", key)
	}
}
