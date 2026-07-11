package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"go-order-management-system/config"
	"go-order-management-system/internal/ordersvc"
	"go-order-management-system/internal/platform/servicehost"
	"go-order-management-system/pkg/database"
)

func main() {
	logger := servicehost.NewLogger("order-reconciliation-worker")
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

	pollInterval, err := durationFromEnv("RECONCILIATION_POLL_INTERVAL", 2*time.Second)
	if err != nil {
		logger.Error("invalid reconciliation poll interval", "error", err)
		os.Exit(1)
	}
	retryDelay, err := durationFromEnv("RECONCILIATION_RETRY_DELAY", 5*time.Second)
	if err != nil {
		logger.Error("invalid reconciliation retry delay", "error", err)
		os.Exit(1)
	}
	maxRetryDelay, err := durationFromEnv("RECONCILIATION_MAX_RETRY_DELAY", 5*time.Minute)
	if err != nil {
		logger.Error("invalid reconciliation max retry delay", "error", err)
		os.Exit(1)
	}
	leaseDuration, err := durationFromEnv("RECONCILIATION_LEASE_DURATION", 30*time.Second)
	if err != nil {
		logger.Error("invalid reconciliation lease duration", "error", err)
		os.Exit(1)
	}
	callTimeout, err := durationFromEnv("RECONCILIATION_CALL_TIMEOUT", 10*time.Second)
	if err != nil {
		logger.Error("invalid reconciliation call timeout", "error", err)
		os.Exit(1)
	}
	batchSize, err := positiveIntFromEnv("RECONCILIATION_BATCH_SIZE", 10)
	if err != nil {
		logger.Error("invalid reconciliation batch size", "error", err)
		os.Exit(1)
	}

	internalToken := os.Getenv("INTERNAL_SERVICE_TOKEN")
	inventoryClient := ordersvc.NewInventoryClient(os.Getenv("INVENTORY_SERVICE_URL"), internalToken, callTimeout)
	worker, err := ordersvc.NewReconciliationWorker(ordersvc.ReconciliationWorkerConfig{
		WorkerID:      os.Getenv("WORKER_ID"),
		PollInterval:  pollInterval,
		RetryDelay:    retryDelay,
		MaxRetryDelay: maxRetryDelay,
		LeaseDuration: leaseDuration,
		CallTimeout:   callTimeout,
		BatchSize:     batchSize,
	}, db, inventoryClient, logger)
	if err != nil {
		logger.Error("initialize reconciliation worker", "error", err)
		os.Exit(1)
	}

	ctx, stop := servicehost.SignalContext()
	defer stop()
	if err := worker.Run(ctx); err != nil {
		logger.Error("order reconciliation worker stopped", "error", err)
		os.Exit(1)
	}
	logger.Info("order reconciliation worker stopped")
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

func positiveIntFromEnv(key string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("parse %s=%q: %w", key, raw, err)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("%s must be greater than zero", key)
	}
	return parsed, nil
}
