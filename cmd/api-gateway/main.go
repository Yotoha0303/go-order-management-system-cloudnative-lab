package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"go-order-management-system/internal/platform/ratelimit"
	"go-order-management-system/internal/platform/resiliencehttp"
	"go-order-management-system/internal/platform/servicehost"
)

type route struct {
	name   string
	prefix string
	base   *url.URL
	proxy  *httputil.ReverseProxy
}

type gateway struct {
	routes  []route
	client  *http.Client
	limiter *ratelimit.Limiter
}

func main() {
	logger := servicehost.NewLogger("api-gateway")

	routes, err := buildRoutes(map[string]string{
		"identity-service":  envOrDefault("IDENTITY_SERVICE_URL", "http://identity-service:8083"),
		"catalog-service":   envOrDefault("CATALOG_SERVICE_URL", "http://catalog-service:8084"),
		"inventory-service": envOrDefault("INVENTORY_SERVICE_URL", "http://inventory-service:8085"),
		"order-service":     envOrDefault("ORDER_SERVICE_URL", "http://order-service:8086"),
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
		client: resiliencehttp.NewHTTPClient(resiliencehttp.TransportConfig{
			ConnectTimeout:        300 * time.Millisecond,
			ResponseHeaderTimeout: time.Second,
			TotalTimeout:          2 * time.Second,
		}),
		limiter: ratelimit.New(ratelimit.Config{
			PerClientRate:  envPositiveFloat("GATEWAY_RATE_LIMIT_PER_CLIENT_RPS", 50),
			PerClientBurst: envPositiveInt("GATEWAY_RATE_LIMIT_PER_CLIENT_BURST", 100),
			GlobalRate:     envPositiveFloat("GATEWAY_RATE_LIMIT_GLOBAL_RPS", 500),
			GlobalBurst:    envPositiveInt("GATEWAY_RATE_LIMIT_GLOBAL_BURST", 1000),
			MaxClients:     envPositiveInt("GATEWAY_RATE_LIMIT_MAX_CLIENTS", 10000),
			InactiveTTL:    envPositiveDuration("GATEWAY_RATE_LIMIT_INACTIVE_TTL", 10*time.Minute),
			CleanupEvery:   envPositiveDuration("GATEWAY_RATE_LIMIT_CLEANUP_INTERVAL", time.Minute),
		}),
	}
	budgetedHandler := resiliencehttp.BudgetHandler(handler, resiliencehttp.BudgetConfig{
		Default: 10 * time.Second,
		Maximum: 30 * time.Second,
	})
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           budgetedHandler,
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
	proxy.Transport = resiliencehttp.NewTransport(resiliencehttp.TransportConfig{
		ConnectTimeout:        500 * time.Millisecond,
		TLSHandshakeTimeout:   time.Second,
		ResponseHeaderTimeout: 8 * time.Second,
		IdleConnTimeout:       90 * time.Second,
	})
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = target.Host
		resiliencehttp.ApplyMetadata(req.Context(), req)
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, req *http.Request, proxyErr error) {
		status := http.StatusBadGateway
		code := "upstream_unavailable"
		if errors.Is(proxyErr, context.DeadlineExceeded) || errors.Is(req.Context().Err(), context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
			code = "upstream_timeout"
		}
		writeJSON(w, status, map[string]any{
			"code":       code,
			"message":    proxyErr.Error(),
			"request_id": resiliencehttp.RequestID(req.Context()),
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

	if allowed, retryAfter := g.limiter.Allow(clientKey(req)); !allowed {
		seconds := int64(math.Ceil(retryAfter.Seconds()))
		if seconds < 1 {
			seconds = 1
		}
		w.Header().Set("Retry-After", strconv.FormatInt(seconds, 10))
		writeJSON(w, http.StatusTooManyRequests, map[string]any{
			"code":        "rate_limited",
			"message":     "request rate limit exceeded",
			"request_id":  resiliencehttp.RequestID(req.Context()),
			"retry_after": seconds,
		})
		return
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
		resiliencehttp.ApplyMetadata(req.Context(), healthReq)
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

func clientKey(req *http.Request) string {
	remote := strings.TrimSpace(req.RemoteAddr)
	if host, _, err := net.SplitHostPort(remote); err == nil && host != "" {
		return host
	}
	if remote == "" {
		return "unknown"
	}
	return remote
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

func envPositiveFloat(key string, fallback float64) float64 {
	value := strings.TrimSpace(os.Getenv(key))
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
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

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
