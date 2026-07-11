package ratelimit

import (
	"sync"
	"testing"
	"time"
)

type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func newTestClock() *testClock {
	return &testClock{now: time.Date(2026, time.July, 11, 12, 0, 0, 0, time.UTC)}
}

func (clock *testClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.now
}

func (clock *testClock) Advance(delta time.Duration) {
	clock.mu.Lock()
	clock.now = clock.now.Add(delta)
	clock.mu.Unlock()
}

func TestLimiterBurstAndRefill(t *testing.T) {
	clock := newTestClock()
	limiter := newWithClock(Config{
		PerClientRate:  2,
		PerClientBurst: 2,
		GlobalRate:     100,
		GlobalBurst:    100,
	}, clock.Now)

	for index := 0; index < 2; index++ {
		allowed, wait := limiter.Allow("client-a")
		if !allowed || wait != 0 {
			t.Fatalf("burst request %d was rejected: allowed=%v wait=%s", index+1, allowed, wait)
		}
	}
	allowed, wait := limiter.Allow("client-a")
	if allowed {
		t.Fatal("third request must exceed the client burst")
	}
	if wait != 500*time.Millisecond {
		t.Fatalf("expected 500ms refill wait, got %s", wait)
	}

	clock.Advance(500 * time.Millisecond)
	allowed, wait = limiter.Allow("client-a")
	if !allowed || wait != 0 {
		t.Fatalf("refilled token was not available: allowed=%v wait=%s", allowed, wait)
	}
}

func TestLimiterIsolatesClients(t *testing.T) {
	clock := newTestClock()
	limiter := newWithClock(Config{
		PerClientRate:  1,
		PerClientBurst: 1,
		GlobalRate:     100,
		GlobalBurst:    100,
	}, clock.Now)

	if allowed, _ := limiter.Allow("client-a"); !allowed {
		t.Fatal("first client-a request must be allowed")
	}
	if allowed, _ := limiter.Allow("client-a"); allowed {
		t.Fatal("second client-a request must be limited")
	}
	if allowed, _ := limiter.Allow("client-b"); !allowed {
		t.Fatal("client-b must have an independent bucket")
	}
}

func TestLimiterEnforcesGlobalSafetyCap(t *testing.T) {
	clock := newTestClock()
	limiter := newWithClock(Config{
		PerClientRate:  100,
		PerClientBurst: 100,
		GlobalRate:     1,
		GlobalBurst:    2,
	}, clock.Now)

	if allowed, _ := limiter.Allow("client-a"); !allowed {
		t.Fatal("first global request must be allowed")
	}
	if allowed, _ := limiter.Allow("client-b"); !allowed {
		t.Fatal("second global request must be allowed")
	}
	allowed, wait := limiter.Allow("client-c")
	if allowed {
		t.Fatal("third request must exceed the global burst")
	}
	if wait != time.Second {
		t.Fatalf("expected one-second global retry, got %s", wait)
	}
}

func TestLimiterRemovesInactiveClients(t *testing.T) {
	clock := newTestClock()
	limiter := newWithClock(Config{
		PerClientRate:  10,
		PerClientBurst: 10,
		GlobalRate:     100,
		GlobalBurst:    100,
		InactiveTTL:    time.Second,
		CleanupEvery:   time.Second,
	}, clock.Now)

	_, _ = limiter.Allow("client-a")
	_, _ = limiter.Allow("client-b")
	clock.Advance(2 * time.Second)
	_, _ = limiter.Allow("client-c")

	if len(limiter.clients) != 1 {
		t.Fatalf("expected only the active client after cleanup, got %d", len(limiter.clients))
	}
	if _, ok := limiter.clients["client-c"]; !ok {
		t.Fatal("active client was removed during cleanup")
	}
}

func TestLimiterKeepsClientStateBounded(t *testing.T) {
	clock := newTestClock()
	limiter := newWithClock(Config{
		PerClientRate:  10,
		PerClientBurst: 10,
		GlobalRate:     100,
		GlobalBurst:    100,
		MaxClients:     2,
		InactiveTTL:    time.Hour,
		CleanupEvery:   time.Hour,
	}, clock.Now)

	_, _ = limiter.Allow("client-a")
	clock.Advance(time.Millisecond)
	_, _ = limiter.Allow("client-b")
	clock.Advance(time.Millisecond)
	_, _ = limiter.Allow("client-c")

	if len(limiter.clients) != 2 {
		t.Fatalf("expected bounded client state, got %d", len(limiter.clients))
	}
	if _, ok := limiter.clients["client-a"]; ok {
		t.Fatal("oldest client bucket was not evicted")
	}
}
