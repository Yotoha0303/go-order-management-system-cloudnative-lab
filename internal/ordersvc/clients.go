package ordersvc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

var (
	errInvalidReservationIdentity = errors.New("inventory reservation identity is required")
	serviceRetryPolicy            = resiliencehttp.RetryPolicy{
		MaxAttempts:       3,
		BaseBackoff:       50 * time.Millisecond,
		MaxBackoff:        300 * time.Millisecond,
		MinimumAttemptGap: 100 * time.Millisecond,
	}
)

type RemoteError struct {
	Service string
	Status  int
	Body    string
}

func (e *RemoteError) Error() string {
	return fmt.Sprintf("%s returned %d: %s", e.Service, e.Status, e.Body)
}

type CatalogProduct struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	PriceFen    int64  `json:"price_fen"`
	Status      int8   `json:"status"`
}

type CatalogClient struct {
	baseURL  string
	token    string
	executor *resiliencehttp.Executor
}

func NewCatalogClient(baseURL, token string, timeout time.Duration) *CatalogClient {
	return &CatalogClient{
		baseURL:  strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		token:    strings.TrimSpace(token),
		executor: newHTTPExecutor(timeout),
	}
}

func (c *CatalogClient) GetProduct(ctx context.Context, productID int64) (*CatalogProduct, error) {
	endpoint := c.baseURL + "/internal/v1/products/" + strconv.FormatInt(productID, 10)
	var product CatalogProduct
	if err := doJSON(ctx, c.executor, c.token, "catalog-service", "get-product-snapshot", http.MethodGet, endpoint, nil, &product); err != nil {
		return nil, err
	}
	return &product, nil
}

type ReservationItem struct {
	ProductID int64 `json:"product_id"`
	Quantity  int64 `json:"quantity"`
}

type InventoryReservation struct {
	ID      string            `json:"id"`
	OrderID int64             `json:"order_id"`
	Status  string            `json:"status"`
	Items   []ReservationItem `json:"items"`
}

type InventoryClient struct {
	baseURL  string
	token    string
	executor *resiliencehttp.Executor
}

func NewInventoryClient(baseURL, token string, timeout time.Duration) *InventoryClient {
	return &InventoryClient{
		baseURL:  strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		token:    strings.TrimSpace(token),
		executor: newHTTPExecutor(timeout),
	}
}

func (c *InventoryClient) Reserve(ctx context.Context, orderID int64, reservationID string, items []ReservationItem) (*InventoryReservation, error) {
	reservationID = strings.TrimSpace(reservationID)
	if orderID <= 0 || reservationID == "" {
		return nil, errInvalidReservationIdentity
	}
	payload := struct {
		OrderID       int64             `json:"order_id"`
		ReservationID string            `json:"reservation_id"`
		Items         []ReservationItem `json:"items"`
	}{OrderID: orderID, ReservationID: reservationID, Items: items}
	var reservation InventoryReservation
	if err := doJSON(ctx, c.executor, c.token, "inventory-service", "reserve-inventory", http.MethodPost, c.baseURL+"/internal/v1/reservations", payload, &reservation); err != nil {
		return nil, err
	}
	return &reservation, nil
}

func (c *InventoryClient) Confirm(ctx context.Context, reservationID string) (*InventoryReservation, error) {
	return c.transition(ctx, reservationID, "confirm")
}

func (c *InventoryClient) Release(ctx context.Context, reservationID string) (*InventoryReservation, error) {
	return c.transition(ctx, reservationID, "release")
}

func (c *InventoryClient) transition(ctx context.Context, reservationID, action string) (*InventoryReservation, error) {
	reservationID = strings.TrimSpace(reservationID)
	if reservationID == "" {
		return nil, errInvalidReservationIdentity
	}
	endpoint := c.baseURL + "/internal/v1/reservations/" + url.PathEscape(reservationID) + "/" + action
	var reservation InventoryReservation
	if err := doJSON(ctx, c.executor, c.token, "inventory-service", action+"-inventory-reservation", http.MethodPost, endpoint, struct{}{}, &reservation); err != nil {
		return nil, err
	}
	return &reservation, nil
}

type OrderServiceClient struct {
	baseURL  string
	token    string
	executor *resiliencehttp.Executor
}

func NewOrderServiceClient(baseURL, token string, timeout time.Duration) *OrderServiceClient {
	return &OrderServiceClient{
		baseURL:  strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		token:    strings.TrimSpace(token),
		executor: newHTTPExecutor(timeout),
	}
}

func (c *OrderServiceClient) TimeoutCancel(ctx context.Context, orderID int64) error {
	endpoint := c.baseURL + "/internal/v1/orders/" + strconv.FormatInt(orderID, 10) + "/timeout-cancel"
	return doJSON(ctx, c.executor, c.token, "order-service", "timeout-cancel-order", http.MethodPost, endpoint, struct{}{}, nil)
}

func newHTTPExecutor(timeout time.Duration) *resiliencehttp.Executor {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	client := resiliencehttp.NewHTTPClient(resiliencehttp.TransportConfig{
		ConnectTimeout:        500 * time.Millisecond,
		TLSHandshakeTimeout:   time.Second,
		ResponseHeaderTimeout: 2 * time.Second,
		TotalTimeout:          timeout,
	})
	return resiliencehttp.NewExecutor(client, slog.Default().With("component", "service-client"))
}

func doJSON(
	ctx context.Context,
	executor *resiliencehttp.Executor,
	token string,
	service string,
	operation string,
	method string,
	endpoint string,
	payload any,
	output any,
) error {
	if executor == nil || strings.TrimSpace(endpoint) == "" || strings.TrimSpace(token) == "" {
		return fmt.Errorf("%s client is not configured", service)
	}

	var encoded []byte
	if payload != nil {
		var err error
		encoded, err = json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode %s request: %w", service, err)
		}
	}

	resp, err := executor.Do(ctx, service, operation, serviceRetryPolicy, func(attemptCtx context.Context) (*http.Request, error) {
		var body io.Reader
		if payload != nil {
			body = bytes.NewReader(encoded)
		}
		// #nosec G107 -- the URL is supplied by trusted service deployment configuration.
		req, buildErr := http.NewRequestWithContext(attemptCtx, method, endpoint, body)
		if buildErr != nil {
			return nil, fmt.Errorf("build %s request: %w", service, buildErr)
		}
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		internalapi.Set(req, token)
		return req, nil
	})
	if err != nil {
		return fmt.Errorf("call %s: %w", service, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		content, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return &RemoteError{Service: service, Status: resp.StatusCode, Body: strings.TrimSpace(string(content))}
	}
	if output == nil || resp.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(output); err != nil {
		return fmt.Errorf("decode %s response: %w", service, err)
	}
	return nil
}
