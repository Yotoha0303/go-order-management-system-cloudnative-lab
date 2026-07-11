package resiliencehttp

import (
	"errors"
	"io"
	"log/slog"
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

func TestCircuitBreakerPermanentResponseCountsAsHealthyInfrastructure(t *testing.T) {
	clock := newFakeClock()
	breaker := newCircuitBreaker("identity-service/role-check", CircuitBreakerConfig{
		FailureThreshold: 1,
		OpenInterval:     time.Second,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)), clock.Now)

	call, err := breaker.acquire()
	if err != nil {
		t.Fatalf("acquire call: %v", err)
	}
	call(true)

	next, err := breaker.acquire()
	if err != nil {
		t.Fatalf("permanent domain response must not open circuit: %v", err)
	}
	next(true)
}
