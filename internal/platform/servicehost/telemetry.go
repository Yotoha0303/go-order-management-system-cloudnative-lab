package servicehost

import (
	"context"
	"log/slog"
	"time"

	platformtelemetry "go-order-management-system/internal/platform/telemetry"
)

func SetupTelemetry(service string, logger *slog.Logger) func() {
	if logger == nil {
		logger = slog.Default()
	}
	shutdown, err := platformtelemetry.Setup(context.Background(), service)
	if err != nil {
		logger.Warn("initialize trace export", "error", err)
	}
	if shutdown == nil {
		return func() {}
	}
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdown(ctx); err != nil {
			logger.Warn("shutdown tracing", "error", err)
		}
	}
}
