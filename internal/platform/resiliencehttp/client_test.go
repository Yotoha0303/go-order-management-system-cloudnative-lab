package resiliencehttp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestExecutorRetriesTransientStatusesAndPreservesMetadata(t *testing.T) {
	var mu sync.Mutex
	attempts := 0
	requestIDs := make([]string, 0, 3)
	deadlines := make([]string, 0, 3)
	bodies := make([]string, 0, 3)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		mu.Lock()
		attempts++
		current := attempts
		requestIDs = append(requestIDs, req.Header.Get(RequestIDHeader))
		deadlines = append(deadlines, req.Header.Get(DeadlineHeader))
		bodies = append(bodies, string(body))
		mu.Unlock()
		if current < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	executor := NewExecutor(&http.Client{Timeout: time.Second}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	executor.sleep = func(context.Context, time.Duration) error { return nil }
	executor.jitter = func(delay time.Duration) time.Duration { return delay }

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	ctx = context.WithValue(ctx, requestIDContextKey, "retry-request")
	payload := []byte(`{"reservation_id":"stable-reservation"}`)

	resp, err := executor.Do(ctx, "inventory-service", "reserve", RetryPolicy{
		MaxAttempts:       3,
		BaseBackoff:       time.Millisecond,
		MaxBackoff:        2 * time.Millisecond,
		MinimumAttemptGap: time.Millisecond,
	}, func(ctx context.Context) (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodPost, server.URL, bytes.NewReader(payload))
	})
	if err != nil {
		t.Fatalf("execute retrying request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}
	if attempts != 3 {
		t.Fatalf("expected three attempts, got %d", attempts)
	}
	for index := range requestIDs {
		if requestIDs[index] != "retry-request" {
			t.Fatalf("attempt %d lost request id: %q", index+1, requestIDs[index])
		}
		if deadlines[index] == "" {
			t.Fatalf("attempt %d lost deadline header", index+1)
		}
		if bodies[index] != string(payload) {
			t.Fatalf("attempt %d changed stable payload: %q", index+1, bodies[index])
		}
	}
}

func TestExecutorDoesNotRetryPermanentStatus(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	executor := NewExecutor(&http.Client{Timeout: time.Second}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	executor.sleep = func(context.Context, time.Duration) error { return nil }

	resp, err := executor.Do(context.Background(), "catalog-service", "snapshot", RetryPolicy{MaxAttempts: 3}, func(ctx context.Context) (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	})
	if err != nil {
		t.Fatalf("permanent response returned transport error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	if attempts != 1 {
		t.Fatalf("expected one attempt, got %d", attempts)
	}
}

func TestExecutorStopsWhenBudgetCannotCoverAnotherAttempt(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	executor := NewExecutor(&http.Client{Timeout: time.Second}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	executor.jitter = func(delay time.Duration) time.Duration { return delay }
	executor.sleep = func(context.Context, time.Duration) error {
		t.Fatal("sleep must not run when budget is insufficient")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	resp, err := executor.Do(ctx, "catalog-service", "snapshot", RetryPolicy{
		MaxAttempts:       3,
		BaseBackoff:       15 * time.Millisecond,
		MaxBackoff:        15 * time.Millisecond,
		MinimumAttemptGap: 10 * time.Millisecond,
	}, func(ctx context.Context) (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	})
	if err != nil {
		t.Fatalf("expected final retryable response, got transport error %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", resp.StatusCode)
	}
	if attempts != 1 {
		t.Fatalf("expected one attempt, got %d", attempts)
	}
}

func TestExecutorEnforcesSlowUpstreamDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	executor := NewExecutor(&http.Client{Timeout: time.Second}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	resp, err := executor.Do(ctx, "slow-service", "slow-call", RetryPolicy{MaxAttempts: 3}, func(ctx context.Context) (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	})
	if resp != nil {
		_ = resp.Body.Close()
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}
