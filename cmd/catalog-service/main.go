package main

import (
	"os"
	"time"

	"go-order-management-system/config"
	"go-order-management-system/internal/auth"
	"go-order-management-system/internal/bizcache"
	"go-order-management-system/internal/handler"
	"go-order-management-system/internal/middleware"
	"go-order-management-system/internal/platform/servicehost"
	"go-order-management-system/internal/service"
	"go-order-management-system/pkg/database"
	redisstore "go-order-management-system/pkg/redis"

	"github.com/gin-gonic/gin"
)

func main() {
	logger := servicehost.NewLogger("catalog-service")
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

	redisClient, err := redisstore.InitRedis(cfg)
	if err != nil {
		logger.Warn("redis unavailable; product cache disabled", "error", err)
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

	productService := service.NewProductService(db, bizcache.NewProductCache(redisClient))
	productHandler := handler.NewProductHandler(productService)
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
	api.GET("/products", productHandler.ListProducts)
	api.GET("/products/:id", productHandler.GetProductByID)

	admin := api.Group("")
	admin.Use(middleware.AdminMiddleware(roleChecker))
	admin.POST("/products", productHandler.CreateProduct)
	admin.PATCH("/products/:id/on-sale", productHandler.OnSaleProduct)
	admin.PATCH("/products/:id/off-sale", productHandler.OffSaleProduct)

	server := servicehost.NewHTTPServer(
		cfg.Server.Port,
		middleware.TimeoutHandler(router, cfg.HttpServer.Server.Timeout),
		cfg.HttpServer.Server,
	)
	if err := servicehost.RunHTTP(logger, server); err != nil {
		logger.Error("catalog service stopped", "error", err)
		os.Exit(1)
	}
}
