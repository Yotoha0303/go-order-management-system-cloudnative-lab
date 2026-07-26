package inventorysvc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	inventoryv1 "go-order-management-system-cloudnative-lab/internal/platform/grpcapi/inventory/v1"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The point of moving to gRPC is that a caller can tell a definitive rejection
// from one worth retrying, so every domain error has to land on the right side
// of that line.
func TestReservationErrorMapsDomainErrors(t *testing.T) {
	cases := []struct {
		err  error
		want codes.Code
	}{
		{ErrInvalidInventoryAmount, codes.InvalidArgument},
		{ErrInventoryNotFound, codes.NotFound},
		{ErrReservationNotFound, codes.NotFound},
		{ErrInsufficientInventory, codes.FailedPrecondition},
		{ErrReservationTransition, codes.FailedPrecondition},
		{context.Canceled, codes.Canceled},
		{context.DeadlineExceeded, codes.DeadlineExceeded},
		{errors.New("some driver failure"), codes.Internal},
	}

	for _, tc := range cases {
		t.Run(tc.err.Error(), func(t *testing.T) {
			if got := status.Code(reservationError(tc.err)); got != tc.want {
				t.Fatalf("expected %s, got %s", tc.want, got)
			}
		})
	}
}

// Reserve wraps the product id into the message, so the mapping has to survive
// wrapping rather than only matching bare sentinel values.
func TestReservationErrorUnwrapsWrappedErrors(t *testing.T) {
	wrapped := fmt.Errorf("%w: product %d", ErrInsufficientInventory, 42)

	if got := status.Code(reservationError(wrapped)); got != codes.FailedPrecondition {
		t.Fatalf("expected FailedPrecondition, got %s", got)
	}
	if msg := status.Convert(reservationError(wrapped)).Message(); !strings.Contains(msg, "product 42") {
		t.Fatalf("expected the product id to survive, got %q", msg)
	}
}

// Storage failures can carry SQL fragments and table names, which have no place
// in a response body.
func TestReservationErrorHidesStorageDetail(t *testing.T) {
	leaky := errors.New("Error 1062: Duplicate entry 'abc' for key 'inventory_reservations.uk_inventory_reservation_order'")

	msg := status.Convert(reservationError(leaky)).Message()
	if strings.Contains(msg, "inventory_reservations") || strings.Contains(msg, "1062") {
		t.Fatalf("storage detail leaked into the status message: %q", msg)
	}
}

func TestReservationStatusToProto(t *testing.T) {
	cases := map[string]inventoryv1.ReservationStatus{
		ReservationPending:   inventoryv1.ReservationStatus_RESERVATION_STATUS_PENDING,
		ReservationConfirmed: inventoryv1.ReservationStatus_RESERVATION_STATUS_CONFIRMED,
		ReservationReleased:  inventoryv1.ReservationStatus_RESERVATION_STATUS_RELEASED,
		"":                   inventoryv1.ReservationStatus_RESERVATION_STATUS_UNSPECIFIED,
		"something-else":     inventoryv1.ReservationStatus_RESERVATION_STATUS_UNSPECIFIED,
	}

	for value, want := range cases {
		if got := reservationStatusToProto(value); got != want {
			t.Fatalf("status %q: expected %s, got %s", value, want, got)
		}
	}
}

func TestReservationToProtoCarriesEveryField(t *testing.T) {
	created := time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)
	updated := created.Add(time.Minute)

	got := reservationToProto(&Reservation{
		ID:        "res-1",
		OrderID:   7,
		Status:    ReservationConfirmed,
		CreatedAt: created,
		UpdatedAt: updated,
		Items: []ReservationItem{
			{ID: 1, ReservationID: "res-1", ProductID: 3, Quantity: 2},
		},
	})

	if got.GetId() != "res-1" || got.GetOrderId() != 7 {
		t.Fatalf("unexpected identity: %+v", got)
	}
	if got.GetStatus() != inventoryv1.ReservationStatus_RESERVATION_STATUS_CONFIRMED {
		t.Fatalf("unexpected status: %s", got.GetStatus())
	}
	if !got.GetCreatedAt().AsTime().Equal(created) || !got.GetUpdatedAt().AsTime().Equal(updated) {
		t.Fatalf("timestamps did not survive: %v / %v", got.GetCreatedAt().AsTime(), got.GetUpdatedAt().AsTime())
	}

	items := got.GetItems()
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	// The HTTP client drops id and reservation_id because its struct omits them.
	// The contract carries them, so the conversion has to populate them.
	if items[0].GetId() != 1 || items[0].GetReservationId() != "res-1" {
		t.Fatalf("item identity was dropped: %+v", items[0])
	}
	if items[0].GetProductId() != 3 || items[0].GetQuantity() != 2 {
		t.Fatalf("unexpected item contents: %+v", items[0])
	}
}

func TestReservationToProtoHandlesNil(t *testing.T) {
	if got := reservationToProto(nil); got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}
