package resiliencehttp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net"
	"net/http"
	"time"
)

var ErrInsufficientBudget = errors.New("insufficient request budget for another HTTP attempt")

type TransportConfig struct {
	ConnectTimeout        time.Duration
	TLSHandshakeTimeout   time.Duration
	ResponseHeaderTimeout time.Duration
	IdleConnTimeout       time.Duration
	TotalTimeout          time.Duration
	MaxIdleConns          int
	MaxIdleConnsPerHost   int
}

func (cfg TransportConfig) normalized() TransportConfig {
	if cfg.ConnectTimeout <= 0 {
		cfg.ConnectTimeout = 500 * time.Millisecond
	}
	if cfg.TLSHandshakeTimeout <= 0 {
		cfg.TLSHandshakeTimeout = time.Second
	}
	if cfg.ResponseHeaderTimeout <= 0 {
		cfg.ResponseHeaderTimeout = 2 * time.Second
	}
	if cfg.IdleConnTimeout <= 0 {
		cfg.IdleConnTimeout = 90 * time.Second
	}
	if cfg.TotalTimeout <= 0 {
		cfg.TotalTimeout = 5 * time.Second
	}
	if cfg.MaxIdleConns <= 0 {
		cfg.MaxIdleConns = 100
	}
	if cfg.MaxIdleConnsPerHost <= 0 {
		cfg.MaxIdleConnsPerHost = 20
	}
	return cfg
}

func NewTransport(cfg TransportConfig) *http.Transport {
	cfg = cfg.normalized()
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: cfg.ConnectTimeout, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:   cfg.TLSHandshakeTimeout,
		ResponseHeaderTimeout: cfg.ResponseHeaderTimeout,
		IdleConnTimeout:       cfg.IdleConnTimeout,
		MaxIdleConns:          cfg.MaxIdleConns,
		MaxIdleConnsPerHost:   cfg.MaxIdleConnsPerHost,
	}
}

func NewHTTPClient(cfg TransportConfig) *http.Client {
	cfg = cfg.normalized()
	return &http.Client{Transport: NewTransport(cfg), Timeout: cfg.TotalTimeout}
}

type RetryPolicy struct {
	MaxAttempts       int
	BaseBackoff       time.Duration
	MaxBackoff        time.Duration
	MinimumAttemptGap time.Duration
}

func (policy RetryPolicy) normalized() RetryPolicy {
	if policy.MaxAttempts <= 0 {
		policy.MaxAttempts = 1
	}
	if policy.BaseBackoff <= 0 {
		policy.BaseBackoff = 50 * time.Millisecond
	}
	if policy.MaxBackoff <= 0 {
		policy.MaxBackoff = 500 * time.Millisecond
	}
	if policy.BaseBackoff > policy.MaxBackoff {
		policy.BaseBackoff = policy.MaxBackoff
	}
	if policy.MinimumAttemptGap <= 0 {
		policy.MinimumAttemptGap = 100 * time.Millisecond
	}
	return policy
}

type RequestFactory func(context.Context) (*http.Request, error)
type SleepFunc func(context.Context, time.Duration) error
type JitterFunc func(time.Duration) time.Duration

type Executor struct {
	client *http.Client
	logger *slog.Logger
	sleep  SleepFunc
	jitter JitterFunc
}

func NewExecutor(client *http.Client, logger *slog.Logger) *Executor {
	if client == nil {
		client = NewHTTPClient(TransportConfig{})
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Executor{
		client: client,
		logger: logger,
		sleep:  sleepWithContext,
		jitter: defaultJitter,
	}
}

func (executor *Executor) Do(
	ctx context.Context,
	upstream string,
	operation string,
	policy RetryPolicy,
	factory RequestFactory,
) (*http.Response, error) {
	if executor == nil || executor.client == nil || factory == nil {
		return nil, errors.New("HTTP retry executor is not configured")
	}
	policy = policy.normalized()

	var lastErr error
	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if attempt > 1 {
			remaining, ok := Remaining(ctx)
			if ok && remaining <= policy.MinimumAttemptGap {
				if lastErr != nil {
					return nil, fmt.Errorf("%w: %v", ErrInsufficientBudget, lastErr)
				}
				return nil, ErrInsufficientBudget
			}
		}

		req, err := factory(ctx)
		if err != nil {
			return nil, err
		}
		ApplyMetadata(ctx, req)

		started := time.Now()
		resp, callErr := executor.client.Do(req)
		retryable := shouldRetry(resp, callErr)
		executor.logAttempt(ctx, upstream, operation, attempt, resp, callErr, retryable, time.Since(started))

		if !retryable || attempt == policy.MaxAttempts {
			return resp, callErr
		}
		lastErr = callErr
		if lastErr == nil && resp != nil {
			lastErr = fmt.Errorf("retryable HTTP status %d", resp.StatusCode)
		}

		delay := executor.jitter(backoff(policy, attempt))
		if remaining, ok := Remaining(ctx); ok && remaining <= delay+policy.MinimumAttemptGap {
			return resp, callErr
		}
		if resp != nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()
		}
		if err := executor.sleep(ctx, delay); err != nil {
			return nil, err
		}
	}
	return nil, lastErr
}

func shouldRetry(resp *http.Response, err error) bool {
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return false
		}
		var netErr net.Error
		return errors.As(err, &netErr)
	}
	if resp == nil {
		return false
	}
	switch resp.StatusCode {
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func backoff(policy RetryPolicy, attempt int) time.Duration {
	delay := policy.BaseBackoff
	for current := 1; current < attempt; current++ {
		if delay >= policy.MaxBackoff/2 {
			return policy.MaxBackoff
		}
		delay *= 2
	}
	if delay > policy.MaxBackoff {
		return policy.MaxBackoff
	}
	return delay
}

func defaultJitter(delay time.Duration) time.Duration {
	if delay <= 0 {
		return 0
	}
	factor := 0.8 + rand.Float64()*0.4 // #nosec G404 -- jitter does not require cryptographic randomness.
	return time.Duration(float64(delay) * factor)
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (executor *Executor) logAttempt(
	ctx context.Context,
	upstream string,
	operation string,
	attempt int,
	resp *http.Response,
	err error,
	retryable bool,
	duration time.Duration,
) {
	status := 0
	if resp != nil {
		status = resp.StatusCode
	}
	remainingMS := int64(-1)
	if remaining, ok := Remaining(ctx); ok {
		remainingMS = remaining.Milliseconds()
	}
	outcome := "success"
	if err != nil {
		outcome = "transport_error"
	} else if status < 200 || status >= 300 {
		outcome = "http_error"
	}
	executor.logger.Info(
		"upstream HTTP attempt",
		"request_id", RequestID(ctx),
		"upstream", upstream,
		"operation", operation,
		"attempt", attempt,
		"outcome", outcome,
		"status", status,
		"retryable", retryable,
		"duration_ms", duration.Milliseconds(),
		"remaining_budget_ms", remainingMS,
		"error", err,
	)
}
