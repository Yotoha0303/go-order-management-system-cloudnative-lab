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
	shutdownTelemetry := servicehost.SetupTelemetry("order-timeout-worker", logger)
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
	indicatorInterval, err := durationFromEnv("ORDER_RELIABILITY_LOG_INTERVAL", time.Minute)
	if err != nil {
		logger.Error("invalid ORDER_RELIABILITY_LOG_INTERVAL", "error", err)
		os.Exit(1)
	}
	stuckThreshold, err := durationFromEnv("ORDER_TRANSIENT_STUCK_THRESHOLD", 5*time.Minute)
	if err != nil {
		logger.Error("invalid ORDER_TRANSIENT_STUCK_THRESHOLD", "error", err)
		os.Exit(1)
	}
	managementTimeout, err := durationFromEnv("RABBITMQ_MANAGEMENT_METRICS_TIMEOUT", 2*time.Second)
	if err != nil {
		logger.Error("invalid RABBITMQ_MANAGEMENT_METRICS_TIMEOUT", "error", err)
		os.Exit(1)
	}
	reliabilityReporter, err := ordersvc.NewReliabilityReporter(db, stuckThreshold)
	if err != nil {
		logger.Error("initialize reliability reporter", "error", err)
		os.Exit(1)
	}
	rabbitMQCollector, err := ordersvc.RabbitMQManagementPrometheusCollector(
		envOrDefault("RABBITMQ_MANAGEMENT_URL", "http://rabbitmq:15672"),
		cfg.RabbitMQ.URL,
		managementTimeout,
	)
	if err != nil {
		logger.Error("initialize RabbitMQ management collector", "error", err)
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
	go ordersvc.RunReliabilityLogLoop(ctx, reliabilityReporter, indicatorInterval, logger)
	servicehost.StartMetricsHTTP(
		ctx,
		logger,
		"order-timeout-worker",
		envOrDefault("METRICS_ADDR", ":9091"),
		ordersvc.ReliabilityPrometheusCollector(reliabilityReporter),
		rabbitMQCollector,
	)

	logger.Info(
		"order timeout worker starting",
		"reliability_log_interval", indicatorInterval,
		"transient_stuck_threshold", stuckThreshold,
	)
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

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
