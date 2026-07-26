package internalapi

import (
	"context"
	"crypto/subtle"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// MetadataKey carries the shared internal service token over gRPC. It is the
// lowercase form of Header because gRPC normalises metadata keys to lowercase
// and rejects uppercase ones outright.
const MetadataKey = "x-internal-token"

// UnaryServerInterceptor rejects calls that do not present the expected token.
// It mirrors Middleware: a server configured with an empty expected token
// refuses every call rather than allowing all of them.
func UnaryServerInterceptor(expected string) grpc.UnaryServerInterceptor {
	expected = strings.TrimSpace(expected)
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if !authorized(ctx, expected) {
			return nil, status.Error(codes.Unauthenticated, "invalid internal service token")
		}
		return handler(ctx, req)
	}
}

func authorized(ctx context.Context, expected string) bool {
	if expected == "" {
		return false
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return false
	}
	values := md.Get(MetadataKey)
	if len(values) != 1 {
		return false
	}
	actual := values[0]
	// Length is compared first only to satisfy ConstantTimeCompare's equal-length
	// requirement; it leaks the token length exactly as the HTTP middleware does.
	return len(actual) == len(expected) &&
		subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) == 1
}

// WithToken attaches the internal service token to an outgoing call. It is the
// gRPC counterpart of Set.
func WithToken(ctx context.Context, token string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, MetadataKey, token)
}
