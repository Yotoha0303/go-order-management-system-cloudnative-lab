package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"go-order-management-system/config"
	"go-order-management-system/internal/ordersvc"
	"go-order-management-system/internal/platform/servicehost"
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

	leaseDuration, err := durationFromEnv("OUTBOX_LEASE_DURATION", 30*time.Second)
	if err != nil {
		logger.Error("invalid OUTBOX_LEASE_DURATION", "error", err)
		os.Exit(1)
	}
	publishConfirmTimeout, err := durationFromEnv("RABBITMQ_PUBLISH_CONFIRM_TIMEOUT", 5*time.Second)
	if err != nil {
		logger.Error("invalid RABBITMQ_PUBLISH_CONFIRM_TIMEOUT", "error", err)
		os.Exit(1)
	}

	worker, err := ordersvc.NewWorker(ordersvc.WorkerConfig{
		URL:                   cfg.RabbitMQ.URL,
		ReconnectDelay:        cfg.RabbitMQ.ReconnectDelay,
		PollInterval:          cfg.RabbitMQ.OrderTimeout.OutboxPollInterval,
		RetryDelay:            cfg.RabbitMQ.OrderTimeout.OutboxRetryDelay,
		BatchSize:             cfg.RabbitMQ.OrderTimeout.PublishBatchSize,
		Prefetch:              cfg.RabbitMQ.OrderTimeout.ConsumerPrefetch,
		OrderServiceURL:       os.Getenv("ORDER_SERVICE_URL"),
		InternalToken:         os.Getenv("INTERNAL_SERVICE_TOKEN"),
		CallTimeout:           10 * time.Second,
		WorkerID:              os.Getenv("WORKER_ID"),
		LeaseDuration:         leaseDuration,
		PublishConfirmTimeout: publishConfirmTimeout,
	}, db, logger)
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
