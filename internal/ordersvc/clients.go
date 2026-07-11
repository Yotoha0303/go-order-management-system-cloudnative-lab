package ordersvc

import (
	"bytes"
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
	baseURL string
	token   string
	client  *http.Client
}

func NewCatalogClient(baseURL, token string, timeout time.Duration) *CatalogClient {
	return &CatalogClient{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		token:   strings.TrimSpace(token),
		client:  newHTTPClient(timeout),
	}
}

func (c *CatalogClient) GetProduct(ctx context.Context, productID int64) (*CatalogProduct, error) {
	endpoint := c.baseURL + "/internal/v1/products/" + strconv.FormatInt(productID, 10)
	var product CatalogProduct
	if err := doJSON(ctx, c.client, c.token, "catalog-service", http.MethodGet, endpoint, nil, &product); err != nil {
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
	baseURL string
	token   string
	client  *http.Client
}

func NewInventoryClient(baseURL, token string, timeout time.Duration) *InventoryClient {
	return &InventoryClient{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		token:   strings.TrimSpace(token),
		client:  newHTTPClient(timeout),
	}
}

func (c *InventoryClient) Reserve(ctx context.Context, orderID int64, reservationID string, items []ReservationItem) (*InventoryReservation, error) {
	payload := struct {
		OrderID       int64             `json:"order_id"`
		ReservationID string            `json:"reservation_id,omitempty"`
		Items         []ReservationItem `json:"items"`
	}{OrderID: orderID, ReservationID: reservationID, Items: items}
	var reservation InventoryReservation
	if err := doJSON(ctx, c.client, c.token, "inventory-service", http.MethodPost, c.baseURL+"/internal/v1/reservations", payload, &reservation); err != nil {
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
	endpoint := c.baseURL + "/internal/v1/reservations/" + url.PathEscape(reservationID) + "/" + action
	var reservation InventoryReservation
	if err := doJSON(ctx, c.client, c.token, "inventory-service", http.MethodPost, endpoint, struct{}{}, &reservation); err != nil {
		return nil, err
	}
	return &reservation, nil
}

type OrderServiceClient struct {
	baseURL string
	token   string
	client  *http.Client
}

func NewOrderServiceClient(baseURL, token string, timeout time.Duration) *OrderServiceClient {
	return &OrderServiceClient{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		token:   strings.TrimSpace(token),
		client:  newHTTPClient(timeout),
	}
}

func (c *OrderServiceClient) TimeoutCancel(ctx context.Context, orderID int64) error {
	endpoint := c.baseURL + "/internal/v1/orders/" + strconv.FormatInt(orderID, 10) + "/timeout-cancel"
	return doJSON(ctx, c.client, c.token, "order-service", http.MethodPost, endpoint, struct{}{}, nil)
}

func newHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &http.Client{Timeout: timeout}
}

func doJSON(ctx context.Context, client *http.Client, token, service, method, endpoint string, payload any, output any) error {
	if client == nil || strings.TrimSpace(endpoint) == "" || strings.TrimSpace(token) == "" {
		return fmt.Errorf("%s client is not configured", service)
	}

	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode %s request: %w", service, err)
		}
		body = bytes.NewReader(encoded)
	}

	// #nosec G107 -- the URL is supplied by trusted service deployment configuration.
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return fmt.Errorf("build %s request: %w", service, err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	internalapi.Set(req, token)

	resp, err := client.Do(req)
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
