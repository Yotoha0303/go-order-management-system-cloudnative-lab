package resiliencehttp

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, time.July, 11, 12, 0, 0, 0, time.UTC)}
}

func (clock *fakeClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.now
}

func (clock *fakeClock) Advance(delta time.Duration) {
	clock.mu.Lock()
	clock.now = clock.now.Add(delta)
	clock.mu.Unlock()
}

func TestCircuitBreakerClosedOpenHalfOpenClosed(t *testing.T) {
	clock := newFakeClock()
	breaker := newCircuitBreaker("inventory-service/reserve", CircuitBreakerConfig{
		FailureThreshold:  2,
		OpenInterval:      time.Second,
		HalfOpenMaxProbes: 1,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)), clock.Now)

	first, err := breaker.acquire()
	if err != nil {
		t.Fatalf("acquire first closed call: %v", err)
	}
	first(false)

	second, err := breaker.acquire()
	if err != nil {
		t.Fatalf("acquire second closed call: %v", err)
	}
	second(false)

	if _, err := breaker.acquire(); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected open circuit, got %v", err)
	}

	clock.Advance(time.Second)
	probe, err := breaker.acquire()
	if err != nil {
		t.Fatalf("acquire half-open probe: %v", err)
	}
	probe(true)

	closedAgain, err := breaker.acquire()
	if err != nil {
		t.Fatalf("expected circuit to close after successful probe: %v", err)
	}
	closedAgain(true)
}

func TestCircuitBreakerLimitsHalfOpenProbes(t *testing.T) {
	clock := newFakeClock()
	breaker := newCircuitBreaker("catalog-service/snapshot", CircuitBreakerConfig{
		FailureThreshold:  1,
		OpenInterval:      time.Second,
		HalfOpenMaxProbes: 1,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)), clock.Now)

	failure, err := breaker.acquire()
	if err != nil {
		t.Fatalf("acquire initial call: %v", err)
	}
	failure(false)
	clock.Advance(time.Second)

	probe, err := breaker.acquire()
	if err != nil {
		t.Fatalf("acquire first half-open probe: %v", err)
	}
	if _, err := breaker.acquire(); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected second half-open probe to be rejected, got %v", err)
	}
	probe(false)

	if _, err := breaker.acquire(); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected failed probe to reopen circuit, got %v", err)
	}
}

func TestExecutorOpenCircuitPerformsNoNetworkCall(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Body:       io.NopCloser(strings.NewReader("unavailable")),
			Header:     make(http.Header),
		}, nil
	})}
	executor := NewExecutor(client, slog.New(slog.NewTextHandler(io.Discard, nil)))
	executor.breakerConfig = CircuitBreakerConfig{FailureThreshold: 1, OpenInterval: time.Minute, HalfOpenMaxProbes: 1}

	factory := func(ctx context.Context) (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, "http://catalog-service/internal", nil)
	}
	resp, err := executor.Do(context.Background(), "catalog-service", "snapshot", RetryPolicy{MaxAttempts: 1}, factory)
	if err != nil {
		t.Fatalf("first 503 must remain an HTTP response: %v", err)
	}
	_ = resp.Body.Close()

	if _, err := executor.Do(context.Background(), "catalog-service", "snapshot", RetryPolicy{MaxAttempts: 1}, factory); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected circuit-open error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("open circuit must perform no additional network call, got %d calls", calls)
	}
}

func TestExecutorPermanent4xxDoesNotOpenCircuit(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       io.NopCloser(strings.NewReader("bad request")),
			Header:     make(http.Header),
		}, nil
	})}
	executor := NewExecutor(client, slog.New(slog.NewTextHandler(io.Discard, nil)))
	executor.breakerConfig = CircuitBreakerConfig{FailureThreshold: 1, OpenInterval: time.Minute, HalfOpenMaxProbes: 1}

	factory := func(ctx context.Context) (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, "http://identity-service/internal", nil)
	}
	for index := 0; index < 2; index++ {
		resp, err := executor.Do(context.Background(), "identity-service", "role-check", RetryPolicy{MaxAttempts: 1}, factory)
		if err != nil {
			t.Fatalf("4xx call %d returned transport error: %v", index+1, err)
		}
		_ = resp.Body.Close()
	}
	if calls != 2 {
		t.Fatalf("permanent 4xx must not open circuit, got %d calls", calls)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }
