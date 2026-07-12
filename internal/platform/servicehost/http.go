package servicehost

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go-order-management-system/config"
	platformtelemetry "go-order-management-system/internal/platform/telemetry"
)

func NewLogger(service string) *slog.Logger {
	return slog.New(platformtelemetry.NewTraceHandler(slog.NewJSONHandler(os.Stdout, nil))).With("service", service)
}

func NewHTTPServer(port int, handler http.Handler, cfg config.HttpServerConfig) *http.Server {
	return &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           handler,
		ReadTimeout:       cfg.ReadTimeOut,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		MaxHeaderBytes:    cfg.MaxHeaderBytesKib << 10,
	}
}

func SignalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
}

func RunHTTP(logger *slog.Logger, server *http.Server) error {
	ctx, stop := SignalContext()
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("http server starting", "addr", server.Addr)
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("http server stopped: %w", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown http server: %w", err)
	}
	return nil
}
