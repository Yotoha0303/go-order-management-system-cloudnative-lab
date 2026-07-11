package resiliencehttp

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var ErrCircuitOpen = errors.New("HTTP circuit is open")

type CircuitOpenError struct {
	Key        string
	RetryAfter time.Duration
}

func (err *CircuitOpenError) Error() string {
	return fmt.Sprintf("%s: key=%s retry_after=%s", ErrCircuitOpen, err.Key, err.RetryAfter)
}

func (err *CircuitOpenError) Unwrap() error { return ErrCircuitOpen }

type CircuitBreakerConfig struct {
	FailureThreshold  int
	OpenInterval      time.Duration
	HalfOpenMaxProbes int
}

func CircuitBreakerConfigFromEnvironment() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold:  envPositiveInt("HTTP_CIRCUIT_FAILURE_THRESHOLD", 5),
		OpenInterval:      envPositiveDuration("HTTP_CIRCUIT_OPEN_INTERVAL", 5*time.Second),
		HalfOpenMaxProbes: envPositiveInt("HTTP_CIRCUIT_HALF_OPEN_MAX_PROBES", 1),
	}
}

func (cfg CircuitBreakerConfig) normalized() CircuitBreakerConfig {
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = 5
	}
	if cfg.OpenInterval <= 0 {
		cfg.OpenInterval = 5 * time.Second
	}
	if cfg.HalfOpenMaxProbes <= 0 {
		cfg.HalfOpenMaxProbes = 1
	}
	return cfg
}

type circuitState string

const (
	circuitClosed   circuitState = "closed"
	circuitOpen     circuitState = "open"
	circuitHalfOpen circuitState = "half_open"
)

type circuitBreaker struct {
	mu sync.Mutex

	key    string
	cfg    CircuitBreakerConfig
	logger *slog.Logger
	now    func() time.Time

	state             circuitState
	consecutiveFailed int
	openedAt          time.Time
	halfOpenInFlight  int
}

func newCircuitBreaker(key string, cfg CircuitBreakerConfig, logger *slog.Logger, now func() time.Time) *circuitBreaker {
	cfg = cfg.normalized()
	if logger == nil {
		logger = slog.Default()
	}
	if now == nil {
		now = time.Now
	}
	return &circuitBreaker{
		key:    key,
		cfg:    cfg,
		logger: logger,
		now:    now,
		state:  circuitClosed,
	}
}

func (breaker *circuitBreaker) acquire() (func(bool), error) {
	breaker.mu.Lock()
	defer breaker.mu.Unlock()

	now := breaker.now()
	if breaker.state == circuitOpen {
		retryAt := breaker.openedAt.Add(breaker.cfg.OpenInterval)
		if now.Before(retryAt) {
			return nil, &CircuitOpenError{Key: breaker.key, RetryAfter: retryAt.Sub(now)}
		}
		breaker.transitionLocked(circuitHalfOpen)
	}

	probe := breaker.state == circuitHalfOpen
	if probe {
		if breaker.halfOpenInFlight >= breaker.cfg.HalfOpenMaxProbes {
			return nil, &CircuitOpenError{Key: breaker.key, RetryAfter: breaker.cfg.OpenInterval}
		}
		breaker.halfOpenInFlight++
	}

	var once sync.Once
	return func(success bool) {
		once.Do(func() {
			breaker.complete(probe, success)
		})
	}, nil
}

func (breaker *circuitBreaker) complete(probe bool, success bool) {
	breaker.mu.Lock()
	defer breaker.mu.Unlock()

	if probe {
		if breaker.state != circuitHalfOpen {
			return
		}
		if breaker.halfOpenInFlight > 0 {
			breaker.halfOpenInFlight--
		}
		if !success {
			breaker.openLocked()
			return
		}
		if breaker.halfOpenInFlight == 0 {
			breaker.consecutiveFailed = 0
			breaker.transitionLocked(circuitClosed)
		}
		return
	}

	if breaker.state != circuitClosed {
		return
	}
	if success {
		breaker.consecutiveFailed = 0
		return
	}
	breaker.consecutiveFailed++
	if breaker.consecutiveFailed >= breaker.cfg.FailureThreshold {
		breaker.openLocked()
	}
}

func (breaker *circuitBreaker) openLocked() {
	breaker.openedAt = breaker.now()
	breaker.consecutiveFailed = 0
	breaker.halfOpenInFlight = 0
	breaker.transitionLocked(circuitOpen)
}

func (breaker *circuitBreaker) transitionLocked(next circuitState) {
	previous := breaker.state
	if previous == next {
		return
	}
	breaker.state = next
	breaker.logger.Info(
		"HTTP circuit state changed",
		"circuit", breaker.key,
		"from", previous,
		"to", next,
	)
}

func envPositiveInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func envPositiveDuration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
