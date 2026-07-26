package servicehost

import (
	"errors"
	"fmt"
	"log/slog"
	"net"

	"google.golang.org/grpc"
)

// StartGRPC serves server on port in the background and returns a function that
// stops it gracefully.
//
// It does not block, because services run gRPC alongside their HTTP listener
// and RunHTTP owns the signal handling. Call the returned function on the way
// out - deferring it right after this call is enough, since RunHTTP only
// returns once shutdown has begun.
//
// A failure to bind is returned so startup can fail loudly. A later Serve error
// is only logged: by then the caller is blocked inside RunHTTP and has no way
// to receive it.
func StartGRPC(logger *slog.Logger, port int, server *grpc.Server) (func(), error) {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, fmt.Errorf("listen for grpc on port %d: %w", port, err)
	}

	go func() {
		logger.Info("grpc server starting", "addr", listener.Addr().String())
		if err := server.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			logger.Error("grpc server stopped", "error", err)
		}
	}()

	return func() {
		logger.Info("grpc server stopping")
		server.GracefulStop()
	}, nil
}
