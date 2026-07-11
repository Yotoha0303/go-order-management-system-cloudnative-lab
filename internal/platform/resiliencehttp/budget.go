package resiliencehttp

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
)

const (
	DeadlineHeader  = "X-Request-Deadline"
	RequestIDHeader = "X-Request-ID"
)

type contextKey uint8

const requestIDContextKey contextKey = iota

type BudgetConfig struct {
	Default time.Duration
	Maximum time.Duration
}

func (cfg BudgetConfig) normalized() BudgetConfig {
	if cfg.Default <= 0 {
		cfg.Default = 10 * time.Second
	}
	if cfg.Maximum <= 0 {
		cfg.Maximum = 30 * time.Second
	}
	if cfg.Default > cfg.Maximum {
		cfg.Default = cfg.Maximum
	}
	return cfg
}

func BudgetHandler(next http.Handler, cfg BudgetConfig) http.Handler {
	cfg = cfg.normalized()
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		now := time.Now()
		deadline := now.Add(cfg.Default)
		maximumDeadline := now.Add(cfg.Maximum)

		if raw := req.Header.Get(DeadlineHeader); raw != "" {
			if parsed, err := time.Parse(time.RFC3339Nano, raw); err == nil {
				deadline = parsed
				if deadline.After(maximumDeadline) {
					deadline = maximumDeadline
				}
			}
		}
		if parentDeadline, ok := req.Context().Deadline(); ok && parentDeadline.Before(deadline) {
			deadline = parentDeadline
		}

		requestID := req.Header.Get(RequestIDHeader)
		if requestID == "" {
			requestID = "req-" + uuid.NewString()
		}
		w.Header().Set(RequestIDHeader, requestID)

		if !deadline.After(now) {
			writeDeadlineExceeded(w, requestID)
			return
		}

		ctx, cancel := context.WithDeadline(req.Context(), deadline)
		defer cancel()
		ctx = context.WithValue(ctx, requestIDContextKey, requestID)
		req = req.WithContext(ctx)
		req.Header.Set(RequestIDHeader, requestID)
		req.Header.Set(DeadlineHeader, deadline.UTC().Format(time.RFC3339Nano))
		next.ServeHTTP(w, req)
	})
}

func ApplyMetadata(ctx context.Context, req *http.Request) {
	if req == nil {
		return
	}
	if requestID := RequestID(ctx); requestID != "" {
		req.Header.Set(RequestIDHeader, requestID)
	}
	if deadline, ok := ctx.Deadline(); ok {
		req.Header.Set(DeadlineHeader, deadline.UTC().Format(time.RFC3339Nano))
	}
}

func RequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	requestID, _ := ctx.Value(requestIDContextKey).(string)
	return requestID
}

func Remaining(ctx context.Context) (time.Duration, bool) {
	if ctx == nil {
		return 0, false
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		return 0, false
	}
	return time.Until(deadline), true
}

func writeDeadlineExceeded(w http.ResponseWriter, requestID string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusGatewayTimeout)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"code":       "request_deadline_exceeded",
		"message":    "request deadline exceeded",
		"request_id": requestID,
	})
}
