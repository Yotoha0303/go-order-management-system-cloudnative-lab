package main

import (
	"os"

	"go-order-management-system/config"
	"go-order-management-system/internal/ordertimeout"
	"go-order-management-system/internal/platform/servicehost"
	"go-order-management-system/internal/service"
	"go-order-management-system/pkg/database"
)

func main() {
	logger := servicehost.NewLogger("order-timeout-worker")
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

	orderService := service.NewOrderServiceWithTimeout(db, cfg.RabbitMQ.OrderTimeout.Delay)
	worker, err := ordertimeout.NewWorker(ordertimeout.Config{
		URL:                cfg.RabbitMQ.URL,
		ConnectTimeout:     cfg.RabbitMQ.ConnectTimeout,
		ReconnectDelay:     cfg.RabbitMQ.ReconnectDelay,
		OutboxPollInterval: cfg.RabbitMQ.OrderTimeout.OutboxPollInterval,
		OutboxRetryDelay:   cfg.RabbitMQ.OrderTimeout.OutboxRetryDelay,
		PublishBatchSize:   cfg.RabbitMQ.OrderTimeout.PublishBatchSize,
		ConsumerPrefetch:   cfg.RabbitMQ.OrderTimeout.ConsumerPrefetch,
	}, db, orderService, logger)
	if err != nil {
		logger.Error("initialize timeout worker", "error", err)
		os.Exit(1)
	}

	ctx, stop := servicehost.SignalContext()
	defer stop()
	logger.Info("order timeout worker starting")
	if err := worker.Run(ctx); err != nil {
		logger.Error("order timeout worker stopped", "error", err)
		os.Exit(1)
	}
	logger.Info("order timeout worker stopped")
}
