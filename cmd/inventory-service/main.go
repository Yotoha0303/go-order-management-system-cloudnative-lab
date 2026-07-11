package main

import (
	"os"
	"time"

	"go-order-management-system/config"
	"go-order-management-system/internal/auth"
	"go-order-management-system/internal/handler"
	"go-order-management-system/internal/middleware"
	"go-order-management-system/internal/platform/servicehost"
	"go-order-management-system/internal/service"
	"go-order-management-system/pkg/database"

	"github.com/gin-gonic/gin"
)

func main() {
	logger := servicehost.NewLogger("inventory-service")
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

	inventoryHandler := handler.NewInventoryHandler(service.NewInventoryService(db))
	stockLogHandler := handler.NewStockLogHandler(service.NewStockLogService(db))
	healthHandler := handler.NewHealthHandler(db)
	roleChecker := service.NewAuthorizationService(db)

	router := gin.New()
	router.Use(
		middleware.RequestID(),
		middleware.AccessLog(logger),
		middleware.Recovery(logger),
	)

	router.GET("/ping", healthHandler.PingHandler)
	router.GET("/live", healthHandler.LiveHandler)
	router.GET("/readyz", healthHandler.ReadyzHandler)

	api := router.Group("/api/v1")
	api.Use(middleware.AuthMiddleware(tokenManager))
	api.GET("/inventory/products/:product_id", inventoryHandler.GetInventoryByProductID)

	admin := api.Group("")
	admin.Use(middleware.AdminMiddleware(roleChecker))
	admin.POST("/inventory/init", inventoryHandler.InitInventory)
	admin.POST("/inventory/add", inventoryHandler.AddInventory)
	admin.GET("/stock-logs", stockLogHandler.ListStockLogs)

	server := servicehost.NewHTTPServer(
		cfg.Server.Port,
		middleware.TimeoutHandler(router, cfg.HttpServer.Server.Timeout),
		cfg.HttpServer.Server,
	)
	if err := servicehost.RunHTTP(logger, server); err != nil {
		logger.Error("inventory service stopped", "error", err)
		os.Exit(1)
	}
}
