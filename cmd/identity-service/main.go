package main

import (
	"net/http"
	"os"
	"strconv"
	"time"

	"go-order-management-system/config"
	"go-order-management-system/internal/auth"
	"go-order-management-system/internal/handler"
	"go-order-management-system/internal/middleware"
	"go-order-management-system/internal/platform/internalapi"
	"go-order-management-system/internal/platform/resiliencehttp"
	"go-order-management-system/internal/platform/servicehost"
	"go-order-management-system/internal/service"
	"go-order-management-system/pkg/database"

	"github.com/gin-gonic/gin"
)

func main() {
	logger := servicehost.NewLogger("identity-service")
	shutdownTelemetry := servicehost.SetupTelemetry("identity-service", logger)
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

	userHandler := handler.NewUserHandler(service.NewUserService(db), tokenManager)
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
	authRoutes := api.Group("/auth")
	authRoutes.POST("/register", userHandler.Register)
	authRoutes.POST("/login", userHandler.Login)

	protected := api.Group("")
	protected.Use(middleware.AuthMiddleware(tokenManager))
	users := protected.Group("/users")
	users.GET("/me", userHandler.Me)
	users.PUT("/me/profile", userHandler.UpdateProfile)
	users.PATCH("/me/password", userHandler.UpdatePassword)

	internal := router.Group("/internal/v1")
	internal.Use(internalapi.Middleware(os.Getenv("INTERNAL_SERVICE_TOKEN")))
	internal.GET("/users/:id/roles/:role", func(c *gin.Context) {
		userID, parseErr := strconv.ParseInt(c.Param("id"), 10, 64)
		if parseErr != nil || userID <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
			return
		}
		allowed, roleErr := roleChecker.HasRole(c.Request.Context(), userID, c.Param("role"))
		if roleErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "role check failed"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"allowed": allowed})
	})

	applicationHandler := middleware.TimeoutHandler(router, cfg.HttpServer.Server.Timeout)
	budgetedHandler := resiliencehttp.BudgetHandler(applicationHandler, resiliencehttp.BudgetConfig{
		Default: cfg.HttpServer.Server.Timeout,
		Maximum: 30 * time.Second,
	})
	server := servicehost.NewObservedHTTPServer(
		"identity-service",
		cfg.Server.Port,
		budgetedHandler,
		cfg.HttpServer.Server,
	)
	if err := servicehost.RunHTTP(logger, server); err != nil {
		logger.Error("identity service stopped", "error", err)
		os.Exit(1)
	}
}
