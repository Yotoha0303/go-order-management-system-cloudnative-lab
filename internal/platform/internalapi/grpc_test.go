package internalapi

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func invoke(t *testing.T, expected string, ctx context.Context) (bool, codes.Code) {
	t.Helper()

	called := false
	interceptor := UnaryServerInterceptor(expected)
	_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, func(context.Context, any) (any, error) {
		called = true
		return nil, nil
	})
	return called, status.Code(err)
}

func TestUnaryServerInterceptorAcceptsMatchingToken(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(MetadataKey, "expected-token"))

	called, code := invoke(t, "expected-token", ctx)
	if !called {
		t.Fatal("expected the handler to run")
	}
	if code != codes.OK {
		t.Fatalf("expected OK, got %s", code)
	}
}

func TestUnaryServerInterceptorRejectsBadToken(t *testing.T) {
	cases := map[string]context.Context{
		"no metadata at all": context.Background(),
		"missing key":        metadata.NewIncomingContext(context.Background(), metadata.Pairs("other", "expected-token")),
		"wrong value":        metadata.NewIncomingContext(context.Background(), metadata.Pairs(MetadataKey, "wrong-token")),
		"same length":        metadata.NewIncomingContext(context.Background(), metadata.Pairs(MetadataKey, "expected-tokeM")),
		"empty value":        metadata.NewIncomingContext(context.Background(), metadata.Pairs(MetadataKey, "")),
		// Two values are rejected rather than checking either, so a caller cannot
		// smuggle a valid token past a bad one.
		"repeated key": metadata.NewIncomingContext(context.Background(), metadata.MD{
			MetadataKey: []string{"wrong-token", "expected-token"},
		}),
	}

	for name, ctx := range cases {
		t.Run(name, func(t *testing.T) {
			called, code := invoke(t, "expected-token", ctx)
			if called {
				t.Fatal("expected the handler to be skipped")
			}
			if code != codes.Unauthenticated {
				t.Fatalf("expected Unauthenticated, got %s", code)
			}
		})
	}
}

// An unconfigured server must refuse everything rather than accept everything,
// matching Middleware. Getting this backwards would leave the internal API open.
func TestUnaryServerInterceptorFailsClosedWhenUnconfigured(t *testing.T) {
	for name, expected := range map[string]string{"empty": "", "blank": "   "} {
		t.Run(name, func(t *testing.T) {
			ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(MetadataKey, ""))

			called, code := invoke(t, expected, ctx)
			if called {
				t.Fatal("expected the handler to be skipped")
			}
			if code != codes.Unauthenticated {
				t.Fatalf("expected Unauthenticated, got %s", code)
			}
		})
	}
}

func TestWithTokenAttachesMetadata(t *testing.T) {
	ctx := WithToken(context.Background(), "expected-token")

	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		t.Fatal("expected outgoing metadata")
	}
	if got := md.Get(MetadataKey); len(got) != 1 || got[0] != "expected-token" {
		t.Fatalf("unexpected metadata: %v", got)
	}
}
