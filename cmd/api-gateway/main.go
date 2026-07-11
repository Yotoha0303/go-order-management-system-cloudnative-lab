package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"go-order-management-system/internal/platform/servicehost"
)

type route struct {
	name   string
	prefix string
	base   *url.URL
	proxy  *httputil.ReverseProxy
}

type gateway struct {
	routes []route
	client *http.Client
}

func main() {
	logger := servicehost.NewLogger("api-gateway")

	routes, err := buildRoutes(map[string]string{
		"identity-service": envOrDefault("IDENTITY_SERVICE_URL", "http://identity-service:8083"),
		"catalog-service":  envOrDefault("CATALOG_SERVICE_URL", "http://catalog-service:8084"),
		"inventory-service": envOrDefault("INVENTORY_SERVICE_URL", "http://inventory-service:8085"),
		"order-service":    envOrDefault("ORDER_SERVICE_URL", "http://order-service:8086"),
	})
	if err != nil {
		logger.Error("build gateway routes", "error", err)
		os.Exit(1)
	}

	port, err := strconv.Atoi(envOrDefault("GATEWAY_PORT", "8082"))
	if err != nil || port <= 0 {
		logger.Error("invalid GATEWAY_PORT", "value", os.Getenv("GATEWAY_PORT"))
		os.Exit(1)
	}

	handler := &gateway{
		routes: routes,
		client: &http.Client{Timeout: 2 * time.Second},
	}
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           handler,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
		ReadHeaderTimeout: 2 * time.Second,
		MaxHeaderBytes:    128 << 10,
	}
	if err := servicehost.RunHTTP(logger, server); err != nil {
		logger.Error("api gateway stopped", "error", err)
		os.Exit(1)
	}
}

func buildRoutes(services map[string]string) ([]route, error) {
	identity, err := newUpstream("identity-service", services["identity-service"])
	if err != nil {
		return nil, err
	}
	catalog, err := newUpstream("catalog-service", services["catalog-service"])
	if err != nil {
		return nil, err
	}
	inventory, err := newUpstream("inventory-service", services["inventory-service"])
	if err != nil {
		return nil, err
	}
	ordering, err := newUpstream("order-service", services["order-service"])
	if err != nil {
		return nil, err
	}

	return []route{
		identity.withPrefix("/api/v1/auth"),
		identity.withPrefix("/api/v1/users"),
		catalog.withPrefix("/api/v1/products"),
		inventory.withPrefix("/api/v1/inventory"),
		inventory.withPrefix("/api/v1/stock-logs"),
		ordering.withPrefix("/api/v1/orders"),
	}, nil
}

func newUpstream(name, rawURL string) (route, error) {
	target, err := url.Parse(rawURL)
	if err != nil {
		return route{}, fmt.Errorf("parse %s URL: %w", name, err)
	}
	if target.Scheme == "" || target.Host == "" {
		return route{}, fmt.Errorf("invalid %s URL %q", name, rawURL)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = target.Host
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, proxyErr error) {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"code":    "upstream_unavailable",
			"message": proxyErr.Error(),
		})
	}
	return route{name: name, base: target, proxy: proxy}, nil
}

func (r route) withPrefix(prefix string) route {
	r.prefix = prefix
	return r
}

func (g *gateway) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch req.URL.Path {
	case "/ping":
		writeJSON(w, http.StatusOK, map[string]string{"message": "success"})
		return
	case "/live":
		writeJSON(w, http.StatusOK, map[string]string{"message": "live"})
		return
	case "/readyz":
		g.ready(w, req)
		return
	}

	if req.Header.Get("X-Request-ID") == "" {
		req.Header.Set("X-Request-ID", fmt.Sprintf("gw-%d", time.Now().UnixNano()))
	}
	for _, candidate := range g.routes {
		if pathMatches(req.URL.Path, candidate.prefix) {
			candidate.proxy.ServeHTTP(w, req)
			return
		}
	}

	writeJSON(w, http.StatusNotFound, map[string]any{
		"code":    "route_not_found",
		"message": "gateway route not found",
	})
}

func (g *gateway) ready(w http.ResponseWriter, req *http.Request) {
	seen := make(map[string]struct{})
	for _, candidate := range g.routes {
		if _, ok := seen[candidate.name]; ok {
			continue
		}
		seen[candidate.name] = struct{}{}

		healthURL := candidate.base.ResolveReference(&url.URL{Path: "/live"})
		healthReq, err := http.NewRequestWithContext(req.Context(), http.MethodGet, healthURL.String(), nil)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready", "service": candidate.name})
			return
		}
		resp, err := g.client.Do(healthReq)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready", "service": candidate.name})
			return
		}
		_ = resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready", "service": candidate.name})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func pathMatches(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
