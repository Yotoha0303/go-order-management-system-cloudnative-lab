package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type stubDatabasePinger struct {
	err error
}

func (s stubDatabasePinger) PingContext(context.Context) error {
	return s.err
}

func TestReadyzHandler(t *testing.T) {
	tests := []struct {
		name       string
		handler    *HealthHandler
		wantStatus int
		wantCode   int
		wantReady  bool
	}{
		{
			name:       "database is ready",
			handler:    &HealthHandler{db: stubDatabasePinger{}},
			wantStatus: http.StatusOK,
			wantCode:   0,
			wantReady:  true,
		},
		{
			name:       "database is not initialized",
			handler:    &HealthHandler{},
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   5001,
		},
		{
			name:       "database ping fails",
			handler:    &HealthHandler{db: stubDatabasePinger{err: errors.New("ping failed")}},
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   5001,
		},
	}

	gin.SetMode(gin.TestMode)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/readyz", tt.handler.ReadyzHandler)

			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
			router.ServeHTTP(recorder, request)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("expected HTTP status %d, got %d", tt.wantStatus, recorder.Code)
			}

			var body struct {
				Code int `json:"code"`
				Data struct {
					Status string `json:"status"`
				} `json:"data"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if body.Code != tt.wantCode {
				t.Fatalf("expected response code %d, got %d", tt.wantCode, body.Code)
			}
			if tt.wantReady && body.Data.Status != "ready" {
				t.Fatalf("expected ready status, got %q", body.Data.Status)
			}
		})
	}
}
