package app

import (
	"log/slog"
	"os"
	"time"

	"go-order-management-system/config"
	"go-order-management-system/internal/auth"
	"go-order-management-system/internal/bizcache"
	"go-order-management-system/internal/handler"
	"go-order-management-system/internal/service"
	"go-order-management-system/pkg/database"
	"go-order-management-system/pkg/redis"
	"go-order-management-system/router"

	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type Deps struct {
	Config       *config.Config
	DB           *gorm.DB
	RedisClient  *goredis.Client
	Router       *gin.Engine
	Logger       *slog.Logger
	TokenManager *auth.TokenManager
}

func InitDeps(logger *slog.Logger) (*Deps, error) {
	config.LoadEnv()

	cfg, err := config.LoadConfig("config.yml")
	if err != nil {
		return nil, err
	}

	db, err := database.InitDB(cfg)
	if err != nil {
		return nil, err
	}

	redisClient, err := redis.InitRedis(cfg)
	if err != nil {
		logger.Warn("redis unavailable, product cache disabled", "err", err)
	}

	productCache := bizcache.NewProductCache(redisClient)

	productService := service.NewProductService(db, productCache)
	inventoryService := service.NewInventoryService(db)
	stockLogService := service.NewStockLogService(db)
	orderService := service.NewOrderService(db)
	userService := service.NewUserService(db)

	tokenManager, err := auth.NewTokenManager(
		os.Getenv("JWT_SECRET"),
		"go-order-management-system",
		time.Duration(cfg.JWT.ExpireHours)*time.Hour,
	)
	if err != nil {
		return nil, err
	}

	handlers := router.Handlers{
		Product:   handler.NewProductHandler(productService),
		Inventory: handler.NewInventoryHandler(inventoryService),
		StockLog:  handler.NewStockLogHandler(stockLogService),
		Order:     handler.NewOrderHandler(orderService),
		Health:    handler.NewHealthHandler(db),
		User:      handler.NewUserHandler(userService, tokenManager),
	}

	r := router.SetupRouters(logger, handlers, tokenManager)

	return &Deps{
		Config:       cfg,
		DB:           db,
		RedisClient:  redisClient,
		Router:       r,
		Logger:       logger,
		TokenManager: tokenManager,
	}, nil
}
