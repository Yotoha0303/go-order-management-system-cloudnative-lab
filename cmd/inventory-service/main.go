package main

import (
	"os"
	"time"

	"go-order-management-system/config"
	"go-order-management-system/internal/auth"
	"go-order-management-system/internal/handler"
	"go-order-management-system/internal/inventorysvc"
	"go-order-management-system/internal/middleware"
	"go-order-management-system/internal/platform/resiliencehttp"
	"go-order-management-system/internal/platform/serviceclient"
	"go-order-management-system/internal/platform/servicehost"
	"go-order-management-system/pkg/database"

	"github.com/gin-gonic/gin"
)

func main() {
	logger := servicehost.NewLogger("inventory-service")
	shutdownTelemetry := servicehost.SetupTelemetry("inventory-service", logger)
	defer shutdownTelemetry()
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

	healthHandler := handler.NewHealthHandler(db)
	roleChecker := serviceclient.NewIdentityRoleChecker(
		os.Getenv("IDENTITY_SERVICE_URL"),
		os.Getenv("INTERNAL_SERVICE_TOKEN"),
		3*time.Second,
	)

	router := gin.New()
	router.Use(
		middleware.RequestID(),
		middleware.AccessLog(logger),
		middleware.Recovery(logger),
	)
	router.GET("/ping", healthHandler.PingHandler)
	router.GET("/live", healthHandler.LiveHandler)
	router.GET("/readyz", healthHandler.ReadyzHandler)

	inventorysvc.RegisterRoutes(
		router,
		tokenManager,
		roleChecker,
		os.Getenv("INTERNAL_SERVICE_TOKEN"),
		inventorysvc.NewService(db),
	)

	applicationHandler := middleware.TimeoutHandler(router, cfg.HttpServer.Server.Timeout)
	budgetedHandler := resiliencehttp.BudgetHandler(applicationHandler, resiliencehttp.BudgetConfig{
		Default: cfg.HttpServer.Server.Timeout,
		Maximum: 30 * time.Second,
	})
	server := servicehost.NewObservedHTTPServer(
		"inventory-service",
		cfg.Server.Port,
		budgetedHandler,
		cfg.HttpServer.Server,
	)
	if err := servicehost.RunHTTP(logger, server); err != nil {
		logger.Error("inventory service stopped", "error", err)
		os.Exit(1)
	}
}
