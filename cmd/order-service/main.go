package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"go-order-management-system/config"
	"go-order-management-system/internal/auth"
	"go-order-management-system/internal/handler"
	"go-order-management-system/internal/middleware"
	"go-order-management-system/internal/ordersvc"
	"go-order-management-system/internal/platform/resiliencehttp"
	"go-order-management-system/internal/platform/servicehost"
	"go-order-management-system/pkg/database"

	"github.com/gin-gonic/gin"
)

func main() {
	logger := servicehost.NewLogger("order-service")
	config.LoadEnv()

	cfg, err := config.LoadConfig("config.yml")
	if err != nil {
		logger.Error("load config", "error", err)
		os.Exit(1)
	}

	db, err := database.InitDB(cfg)
	if err != nil {
		logger.Error("initialize database", "error", err)
		os.Exit(1)
	}

	tokenManager, err := auth.NewTokenManager(
		os.Getenv("JWT_SECRET"),
		"go-order-management-system",
		time.Duration(cfg.JWT.ExpireHours)*time.Hour,
	)
	if err != nil {
		logger.Error("initialize token manager", "error", err)
		os.Exit(1)
	}

	stuckThreshold, err := durationFromEnv("ORDER_TRANSIENT_STUCK_THRESHOLD", 5*time.Minute)
	if err != nil {
		logger.Error("invalid ORDER_TRANSIENT_STUCK_THRESHOLD", "error", err)
		os.Exit(1)
	}
	reliabilityReporter, err := ordersvc.NewReliabilityReporter(db, stuckThreshold)
	if err != nil {
		logger.Error("initialize reliability reporter", "error", err)
		os.Exit(1)
	}

	internalToken := os.Getenv("INTERNAL_SERVICE_TOKEN")
	catalogClient := ordersvc.NewCatalogClient(os.Getenv("CATALOG_SERVICE_URL"), internalToken, 5*time.Second)
	inventoryClient := ordersvc.NewInventoryClient(os.Getenv("INVENTORY_SERVICE_URL"), internalToken, 5*time.Second)
	orderService := ordersvc.NewService(db, catalogClient, inventoryClient, cfg.RabbitMQ.OrderTimeout.Delay)
	healthHandler := handler.NewHealthHandler(db)

	router := gin.New()
	router.Use(
		middleware.RequestID(),
		middleware.AccessLog(logger),
		middleware.Recovery(logger),
	)
	router.GET("/ping", healthHandler.PingHandler)
	router.GET("/live", healthHandler.LiveHandler)
	router.GET("/readyz", healthHandler.ReadyzHandler)
	ordersvc.RegisterRoutes(router, tokenManager, internalToken, orderService)
	ordersvc.RegisterReliabilityRoutes(router, internalToken, reliabilityReporter)

	applicationHandler := middleware.TimeoutHandler(router, cfg.HttpServer.Server.Timeout)
	budgetedHandler := resiliencehttp.BudgetHandler(applicationHandler, resiliencehttp.BudgetConfig{
		Default: cfg.HttpServer.Server.Timeout,
		Maximum: 30 * time.Second,
	})
	server := servicehost.NewObservedHTTPServer(
		"order-service",
		cfg.Server.Port,
		budgetedHandler,
		cfg.HttpServer.Server,
		ordersvc.ReliabilityPrometheusCollector(reliabilityReporter),
	)
	if err := servicehost.RunHTTP(logger, server); err != nil {
		logger.Error("order service stopped", "error", err)
		os.Exit(1)
	}
}

func durationFromEnv(key string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("parse %s=%q: %w", key, raw, err)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("%s must be greater than zero", key)
	}
	return parsed, nil
}
