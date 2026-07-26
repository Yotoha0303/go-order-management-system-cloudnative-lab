package inventorysvc

import (
	"context"
	"net"
	"testing"

	inventoryv1 "go-order-management-system-cloudnative-lab/internal/platform/grpcapi/inventory/v1"
	"go-order-management-system-cloudnative-lab/internal/platform/internalapi"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const testToken = "internal-token-for-tests"

// dialReservationService wires the real interceptor and the real server over an
// in-memory connection, so these tests exercise transport, authentication,
// method routing, request conversion and error mapping together.
//
// The Service is built with a nil database on purpose. Every case below is
// rejected by validation before any query runs, which keeps the test free of
// external dependencies while still covering the whole path down to the domain
// error. A case that did reach storage would panic rather than pass quietly.
func dialReservationService(t *testing.T) inventoryv1.InventoryReservationServiceClient {
	t.Helper()

	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer(
		grpc.UnaryInterceptor(internalapi.UnaryServerInterceptor(testToken)),
	)
	inventoryv1.RegisterInventoryReservationServiceServer(server, NewGRPCServer(NewService(nil)))

	go func() {
		if err := server.Serve(listener); err != nil {
			t.Errorf("serve: %v", err)
		}
	}()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	t.Cleanup(func() {
		_ = conn.Close()
		server.Stop()
		_ = listener.Close()
	})

	return inventoryv1.NewInventoryReservationServiceClient(conn)
}

func authorizedContext(t *testing.T) context.Context {
	t.Helper()
	return internalapi.WithToken(t.Context(), testToken)
}

func TestGRPCRejectsUnauthenticatedCalls(t *testing.T) {
	client := dialReservationService(t)

	calls := map[string]func(context.Context) error{
		"Reserve": func(ctx context.Context) error {
			_, err := client.Reserve(ctx, &inventoryv1.ReserveRequest{OrderId: 1})
			return err
		},
		"Confirm": func(ctx context.Context) error {
			_, err := client.Confirm(ctx, &inventoryv1.ConfirmRequest{ReservationId: "res-1"})
			return err
		},
		"Release": func(ctx context.Context) error {
			_, err := client.Release(ctx, &inventoryv1.ReleaseRequest{ReservationId: "res-1"})
			return err
		},
	}

	for name, call := range calls {
		t.Run(name+" without token", func(t *testing.T) {
			if got := status.Code(call(t.Context())); got != codes.Unauthenticated {
				t.Fatalf("expected Unauthenticated, got %s", got)
			}
		})
		t.Run(name+" with wrong token", func(t *testing.T) {
			ctx := internalapi.WithToken(t.Context(), "not-the-token")
			if got := status.Code(call(ctx)); got != codes.Unauthenticated {
				t.Fatalf("expected Unauthenticated, got %s", got)
			}
		})
	}
}

func TestGRPCReserveRejectsInvalidRequests(t *testing.T) {
	client := dialReservationService(t)
	ctx := authorizedContext(t)

	cases := map[string]*inventoryv1.ReserveRequest{
		"no order id": {Items: []*inventoryv1.ReserveItem{{ProductId: 1, Quantity: 1}}},
		"no items":    {OrderId: 1},
		"zero quantity": {
			OrderId: 1,
			Items:   []*inventoryv1.ReserveItem{{ProductId: 1, Quantity: 0}},
		},
		"negative product id": {
			OrderId: 1,
			Items:   []*inventoryv1.ReserveItem{{ProductId: -1, Quantity: 1}},
		},
	}

	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := client.Reserve(ctx, req)
			if got := status.Code(err); got != codes.InvalidArgument {
				t.Fatalf("expected InvalidArgument, got %s (%v)", got, err)
			}
		})
	}
}

// The contract states that an empty reservation id reports NotFound rather than
// InvalidArgument, matching the HTTP behaviour it replaces.
func TestGRPCTransitionsRejectEmptyReservationID(t *testing.T) {
	client := dialReservationService(t)
	ctx := authorizedContext(t)

	if _, err := client.Confirm(ctx, &inventoryv1.ConfirmRequest{}); status.Code(err) != codes.NotFound {
		t.Fatalf("confirm: expected NotFound, got %s", status.Code(err))
	}
	if _, err := client.Release(ctx, &inventoryv1.ReleaseRequest{ReservationId: "   "}); status.Code(err) != codes.NotFound {
		t.Fatalf("release: expected NotFound, got %s", status.Code(err))
	}
}
