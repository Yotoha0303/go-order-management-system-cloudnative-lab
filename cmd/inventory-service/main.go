package main

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"go-order-management-system-cloudnative-lab/config"
	"go-order-management-system-cloudnative-lab/internal/auth"
	"go-order-management-system-cloudnative-lab/internal/handler"
	"go-order-management-system-cloudnative-lab/internal/inventorysvc"
	"go-order-management-system-cloudnative-lab/internal/middleware"
	inventoryv1 "go-order-management-system-cloudnative-lab/internal/platform/grpcapi/inventory/v1"
	"go-order-management-system-cloudnative-lab/internal/platform/internalapi"
	"go-order-management-system-cloudnative-lab/internal/platform/resiliencehttp"
	"go-order-management-system-cloudnative-lab/internal/platform/serviceclient"
	"go-order-management-system-cloudnative-lab/internal/platform/servicehost"
	"go-order-management-system-cloudnative-lab/pkg/database"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
)

const defaultGRPCPort = 9085

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
		auth.Issuer,
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

	service := inventorysvc.NewService(db)

	inventorysvc.RegisterRoutes(
		router,
		tokenManager,
		roleChecker,
		os.Getenv("INTERNAL_SERVICE_TOKEN"),
		service,
	)

	// Served alongside HTTP, not instead of it. Nothing calls it yet: the
	// reservation contract exists in gRPC form so order-service can be moved
	// over, and until it is, the HTTP endpoints stay authoritative.
	stopGRPC, err := startReservationGRPC(logger, service)
	if err != nil {
		logger.Error("start grpc server", "error", err)
		os.Exit(1)
	}
	defer stopGRPC()

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

// startReservationGRPC serves the reservation contract over gRPC on GRPC_PORT.
//
// The internal token is checked by an interceptor rather than per method, so a
// method added later cannot be left unauthenticated by omission - the same
// reason the HTTP endpoints sit behind a route group.
func startReservationGRPC(logger *slog.Logger, service *inventorysvc.Service) (func(), error) {
	port := defaultGRPCPort
	if v := os.Getenv("GRPC_PORT"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid GRPC_PORT: %w", err)
		}
		port = parsed
	}

	server := grpc.NewServer(
		grpc.UnaryInterceptor(internalapi.UnaryServerInterceptor(os.Getenv("INTERNAL_SERVICE_TOKEN"))),
	)
	inventoryv1.RegisterInventoryReservationServiceServer(server, inventorysvc.NewGRPCServer(service))

	return servicehost.StartGRPC(logger, port, server)
}
