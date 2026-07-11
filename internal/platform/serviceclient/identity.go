package serviceclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go-order-management-system/internal/platform/internalapi"
	"go-order-management-system/internal/platform/resiliencehttp"
)

type IdentityRoleChecker struct {
	baseURL  string
	token    string
	executor *resiliencehttp.Executor
}

func NewIdentityRoleChecker(baseURL, token string, timeout time.Duration) *IdentityRoleChecker {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	client := resiliencehttp.NewHTTPClient(resiliencehttp.TransportConfig{
		ConnectTimeout:        500 * time.Millisecond,
		TLSHandshakeTimeout:   time.Second,
		ResponseHeaderTimeout: 1500 * time.Millisecond,
		TotalTimeout:          timeout,
	})
	return &IdentityRoleChecker{
		baseURL:  strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		token:    strings.TrimSpace(token),
		executor: resiliencehttp.NewExecutor(client, slog.Default().With("component", "identity-role-client")),
	}
}

func (c *IdentityRoleChecker) HasRole(ctx context.Context, userID int64, roleName string) (bool, error) {
	if c == nil || c.executor == nil || c.baseURL == "" || c.token == "" {
		return false, fmt.Errorf("identity role checker is not configured")
	}
	if userID <= 0 || strings.TrimSpace(roleName) == "" {
		return false, nil
	}

	endpoint := c.baseURL + "/internal/v1/users/" + strconv.FormatInt(userID, 10) + "/roles/" + url.PathEscape(roleName)
	resp, err := c.executor.Do(ctx, "identity-service", "check-user-role", resiliencehttp.RetryPolicy{
		MaxAttempts:       3,
		BaseBackoff:       50 * time.Millisecond,
		MaxBackoff:        300 * time.Millisecond,
		MinimumAttemptGap: 100 * time.Millisecond,
	}, func(attemptCtx context.Context) (*http.Request, error) {
		// #nosec G107 -- the URL is supplied by trusted service deployment configuration.
		req, buildErr := http.NewRequestWithContext(attemptCtx, http.MethodGet, endpoint, nil)
		if buildErr != nil {
			return nil, fmt.Errorf("build identity role request: %w", buildErr)
		}
		internalapi.Set(req, c.token)
		return req, nil
	})
	if err != nil {
		return false, fmt.Errorf("call identity service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return false, fmt.Errorf("identity service returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload struct {
		Allowed bool `json:"allowed"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return false, fmt.Errorf("decode identity role response: %w", err)
	}
	return payload.Allowed, nil
}
