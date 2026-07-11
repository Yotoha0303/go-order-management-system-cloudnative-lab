package ordersvc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go-order-management-system/internal/platform/internalapi"
)

func TestCatalogClientSendsInternalToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(internalapi.Header) != "internal-token" {
			http.Error(w, "missing internal token", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(CatalogProduct{ID: 7, Name: "product", PriceFen: 100, Status: 1})
	}))
	defer server.Close()

	client := NewCatalogClient(server.URL, "internal-token", time.Second)
	product, err := client.GetProduct(context.Background(), 7)
	if err != nil {
		t.Fatalf("get product: %v", err)
	}
	if product.ID != 7 || product.PriceFen != 100 {
		t.Fatalf("unexpected product: %+v", product)
	}
}
