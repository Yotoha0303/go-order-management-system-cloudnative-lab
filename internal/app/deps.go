package app

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"go-order-management-system/config"
	"go-order-management-system/internal/assistant"
	"go-order-management-system/internal/assistantadapter"
	"go-order-management-system/internal/auth"
	"go-order-management-system/internal/bizcache"
	"go-order-management-system/internal/handler"
	"go-order-management-system/internal/ordertimeout"
	"go-order-management-system/internal/service"
	"go-order-management-system/pkg/database"
	"go-order-management-system/pkg/redis"
	"go-order-management-system/router"

	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type Deps struct {
	Config             *config.Config
	DB                 *gorm.DB
	RedisClient        *goredis.Client
	Router             *gin.Engine
	Logger             *slog.Logger
	TokenManager       *auth.TokenManager
	OrderTimeoutWorker *ordertimeout.Worker
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
	orderService := service.NewOrderServiceWithTimeout(db, cfg.OrderTimeout.Delay)
	orderTimeoutWorker, err := ordertimeout.NewWorker(ordertimeout.Config{
		URL:                cfg.RabbitMQ.URL,
		ConnectTimeout:     cfg.RabbitMQ.ConnectTimeout,
		ReconnectDelay:     cfg.RabbitMQ.ReconnectDelay,
		OutboxPollInterval: cfg.OrderTimeout.OutboxPollInterval,
		OutboxRetryDelay:   cfg.OrderTimeout.OutboxRetryDelay,
		PublishBatchSize:   cfg.OrderTimeout.PublishBatchSize,
		ConsumerPrefetch:   cfg.OrderTimeout.ConsumerPrefetch,
	}, db, orderService, logger)
	if err != nil {
		return nil, fmt.Errorf("build order timeout worker: %w", err)
	}
	userService := service.NewUserService(db)
	authorizationService := service.NewAuthorizationService(db)
	assistantHandler, err := buildAssistantHandler(cfg, db, logger)
	if err != nil {
		return nil, err
	}

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
		Assistant: assistantHandler,
	}

	r := router.SetupRouters(logger, handlers, tokenManager, authorizationService)

	return &Deps{
		Config:             cfg,
		DB:                 db,
		RedisClient:        redisClient,
		Router:             r,
		Logger:             logger,
		TokenManager:       tokenManager,
		OrderTimeoutWorker: orderTimeoutWorker,
	}, nil
}

func buildAssistantHandler(
	cfg *config.Config,
	db *gorm.DB,
	logger *slog.Logger,
) (*handler.AssistantHandler, error) {
	repository, err := assistantadapter.NewMySQLRepository(db)
	if err != nil {
		return nil, fmt.Errorf("build assistant repository: %w", err)
	}
	lowStockTool, err := assistant.NewLowStockTool(repository)
	if err != nil {
		return nil, fmt.Errorf("build low-stock tool: %w", err)
	}
	orderSummaryTool, err := assistant.NewOrderStatusSummaryTool(repository, time.Now)
	if err != nil {
		return nil, fmt.Errorf("build order-summary tool: %w", err)
	}
	registry, err := assistant.NewToolRegistry(lowStockTool, orderSummaryTool)
	if err != nil {
		return nil, fmt.Errorf("build assistant tool registry: %w", err)
	}
	llmClient, err := buildAssistantLLMClient(cfg)
	if err != nil {
		return nil, err
	}
	logPersistTimeout := 500 * time.Millisecond
	if cfg.Assistant.Timeout < logPersistTimeout {
		logPersistTimeout = cfg.Assistant.Timeout
	}
	assistantService, err := assistant.NewAssistantService(assistant.ServiceConfig{
		LLM:               llmClient,
		Registry:          registry,
		CallLogs:          repository,
		Timeout:           cfg.Assistant.Timeout,
		Now:               time.Now,
		NewRequestID:      assistant.GenerateRequestID,
		Logger:            logger,
		LogPersistTimeout: logPersistTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("build assistant service: %w", err)
	}
	assistantHandler, err := handler.NewAssistantHandler(assistantService)
	if err != nil {
		return nil, fmt.Errorf("build assistant handler: %w", err)
	}
	return assistantHandler, nil
}

func buildAssistantLLMClient(cfg *config.Config) (assistant.LLMClient, error) {
	switch cfg.Assistant.LLM.Mode {
	case "mock":
		return assistantadapter.NewRuleBasedClient(), nil
	case "chat_completions":
		client, err := assistantadapter.NewChatCompletionsClient(assistantadapter.ChatCompletionsConfig{
			Endpoint:         cfg.Assistant.LLM.Endpoint,
			APIKey:           cfg.Assistant.LLM.APIKey,
			Model:            cfg.Assistant.LLM.Model,
			Provider:         cfg.Assistant.LLM.Provider,
			MaxResponseBytes: cfg.Assistant.LLM.MaxResponseBytes,
			HTTPClient:       &http.Client{Timeout: cfg.Assistant.Timeout},
		})
		if err != nil {
			return nil, fmt.Errorf("build chat completions client: %w", err)
		}
		return client, nil
	default:
		return nil, fmt.Errorf("build assistant LLM: unsupported mode %q", cfg.Assistant.LLM.Mode)
	}
}
