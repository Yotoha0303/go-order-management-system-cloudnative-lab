package servicehost

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"go-order-management-system/config"
	platformmetrics "go-order-management-system/internal/platform/metrics"
)

func NewObservedHTTPServer(
	service string,
	port int,
	handler http.Handler,
	cfg config.HttpServerConfig,
	collectors ...platformmetrics.Collector,
) *http.Server {
	return NewHTTPServer(port, platformmetrics.InstrumentHTTP(service, handler, collectors...), cfg)
}

func StartMetricsHTTP(
	ctx context.Context,
	logger *slog.Logger,
	service string,
	address string,
	collectors ...platformmetrics.Collector,
) {
	address = strings.TrimSpace(address)
	if address == "" {
		return
	}
	platformmetrics.Default.SetGauge(
		"go_order_worker_up",
		"Whether the current background Worker process is running.",
		platformmetrics.Labels{"worker": service},
		1,
	)
	server := &http.Server{
		Addr:              address,
		Handler:           platformmetrics.Default.Handler(collectors...),
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	go func() {
		logger.Info("metrics server starting", "addr", address)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			platformmetrics.Default.SetGauge(
				"go_order_worker_metrics_listener_up",
				"Whether the background Worker metrics listener is available.",
				platformmetrics.Labels{"worker": service},
				0,
			)
			logger.Error("metrics server stopped", "addr", address, "error", err)
		}
	}()

	platformmetrics.Default.SetGauge(
		"go_order_worker_metrics_listener_up",
		"Whether the background Worker metrics listener is available.",
		platformmetrics.Labels{"worker": service},
		1,
	)
	go func() {
		<-ctx.Done()
		platformmetrics.Default.SetGauge(
			"go_order_worker_up",
			"Whether the current background Worker process is running.",
			platformmetrics.Labels{"worker": service},
			0,
		)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("shutdown metrics server", "error", err)
		}
	}()
}
