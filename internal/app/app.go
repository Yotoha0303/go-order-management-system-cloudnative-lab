package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func Run() error {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	deps, err := InitDeps(logger)
	if err != nil {
		return err
	}

	server := NewHTTPServer(deps)
	workerCtx, stopWorker := context.WithCancel(context.Background())
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		if err := deps.OrderTimeoutWorker.Run(workerCtx); err != nil {
			logger.Error("order timeout worker stopped", "error", err)
		}
	}()

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("server starting", "addr", server.Addr)
		serverErr <- server.ListenAndServe()
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	var runErr error
	select {
	case <-quit:
		logger.Info("shutdown signal received")
	case err := <-serverErr:
		runErr = fmt.Errorf("server stopped: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stopWorker()

	shutdownErr := server.Shutdown(ctx)
	select {
	case <-workerDone:
	case <-ctx.Done():
		logger.Warn("order timeout worker shutdown timed out")
	}
	if runErr != nil {
		return runErr
	}
	return shutdownErr
}
