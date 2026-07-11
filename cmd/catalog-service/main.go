package main

import (
	"os"
	"time"

	"go-order-management-system/config"
	"go-order-management-system/internal/auth"
	"go-order-management-system/internal/catalogsvc"
	"go-order-management-system/internal/handler"
	"go-order-management-system/internal/middleware"
	"go-order-management-system/internal/platform/serviceclient"
	"go-order-management-system/internal/platform/servicehost"
	"go-order-management-system/pkg/database"

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
	if err := catalogsvc.Migrate(db); err != nil {
		logger.Error("migrate catalog database", "error", err)
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

	catalogsvc.RegisterRoutes(
		router,
		tokenManager,
		roleChecker,
		os.Getenv("INTERNAL_SERVICE_TOKEN"),
		catalogsvc.NewService(db),
	)

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
