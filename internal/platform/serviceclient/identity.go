package serviceclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go-order-management-system/internal/platform/internalapi"
)

type IdentityRoleChecker struct {
	baseURL string
	token   string
	client  *http.Client
}

func NewIdentityRoleChecker(baseURL, token string, timeout time.Duration) *IdentityRoleChecker {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return &IdentityRoleChecker{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		token:   strings.TrimSpace(token),
		client:  &http.Client{Timeout: timeout},
	}
}

func (c *IdentityRoleChecker) HasRole(ctx context.Context, userID int64, roleName string) (bool, error) {
	if c == nil || c.client == nil || c.baseURL == "" || c.token == "" {
		return false, fmt.Errorf("identity role checker is not configured")
	}
	if userID <= 0 || strings.TrimSpace(roleName) == "" {
		return false, nil
	}

	endpoint := c.baseURL + "/internal/v1/users/" + strconv.FormatInt(userID, 10) + "/roles/" + url.PathEscape(roleName)
	// #nosec G107 -- the URL is supplied by trusted service deployment configuration.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, fmt.Errorf("build identity role request: %w", err)
	}
	internalapi.Set(req, c.token)

	resp, err := c.client.Do(req)
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
