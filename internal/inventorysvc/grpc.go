package inventorysvc

import (
	"context"
	"errors"

	inventoryv1 "go-order-management-system-cloudnative-lab/internal/platform/grpcapi/inventory/v1"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// GRPCServer serves inventory.v1.InventoryReservationService on top of the same
// Service the HTTP handlers use, so both surfaces share one implementation and
// cannot drift apart.
type GRPCServer struct {
	inventoryv1.UnimplementedInventoryReservationServiceServer

	service *Service
}

func NewGRPCServer(service *Service) *GRPCServer {
	return &GRPCServer{service: service}
}

func (s *GRPCServer) Reserve(ctx context.Context, req *inventoryv1.ReserveRequest) (*inventoryv1.ReserveResponse, error) {
	items := make([]ItemRequest, 0, len(req.GetItems()))
	for _, item := range req.GetItems() {
		items = append(items, ItemRequest{
			ProductID: item.GetProductId(),
			Quantity:  item.GetQuantity(),
		})
	}

	reservation, err := s.service.Reserve(ctx, ReserveRequest{
		OrderID:       req.GetOrderId(),
		ReservationID: req.GetReservationId(),
		Items:         items,
	})
	if err != nil {
		return nil, reservationError(err)
	}
	return &inventoryv1.ReserveResponse{Reservation: reservationToProto(reservation)}, nil
}

func (s *GRPCServer) Confirm(ctx context.Context, req *inventoryv1.ConfirmRequest) (*inventoryv1.ConfirmResponse, error) {
	reservation, err := s.service.Confirm(ctx, req.GetReservationId())
	if err != nil {
		return nil, reservationError(err)
	}
	return &inventoryv1.ConfirmResponse{Reservation: reservationToProto(reservation)}, nil
}

func (s *GRPCServer) Release(ctx context.Context, req *inventoryv1.ReleaseRequest) (*inventoryv1.ReleaseResponse, error) {
	reservation, err := s.service.Release(ctx, req.GetReservationId())
	if err != nil {
		return nil, reservationError(err)
	}
	return &inventoryv1.ReleaseResponse{Reservation: reservationToProto(reservation)}, nil
}

// reservationError maps domain errors onto status codes so callers can tell a
// definitive business rejection from a transport failure. The HTTP surface
// cannot express that distinction: it answers 409 for insufficient stock, an
// unknown product and a malformed request alike, and order-service only ever
// sees the raw response body.
//
// The split that matters to a caller is whether retrying could succeed.
// FailedPrecondition and InvalidArgument will not change on retry, so the saga
// should fail the order. Internal and Unavailable might, so the saga should
// compensate and let the reconciliation worker retry.
func reservationError(err error) error {
	switch {
	case errors.Is(err, ErrInvalidInventoryAmount):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, ErrInventoryNotFound), errors.Is(err, ErrReservationNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, ErrInsufficientInventory), errors.Is(err, ErrReservationTransition):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, err.Error())
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, err.Error())
	default:
		// Deliberately not err.Error(): everything reaching here is a storage
		// failure, whose message can carry SQL fragments and table names.
		return status.Error(codes.Internal, "inventory reservation failed")
	}
}

func reservationToProto(reservation *Reservation) *inventoryv1.Reservation {
	if reservation == nil {
		return nil
	}

	items := make([]*inventoryv1.ReservationItem, 0, len(reservation.Items))
	for _, item := range reservation.Items {
		items = append(items, &inventoryv1.ReservationItem{
			Id:            item.ID,
			ReservationId: item.ReservationID,
			ProductId:     item.ProductID,
			Quantity:      item.Quantity,
		})
	}

	return &inventoryv1.Reservation{
		Id:        reservation.ID,
		OrderId:   reservation.OrderID,
		Status:    reservationStatusToProto(reservation.Status),
		CreatedAt: timestamppb.New(reservation.CreatedAt),
		UpdatedAt: timestamppb.New(reservation.UpdatedAt),
		Items:     items,
	}
}

func reservationStatusToProto(status string) inventoryv1.ReservationStatus {
	switch status {
	case ReservationPending:
		return inventoryv1.ReservationStatus_RESERVATION_STATUS_PENDING
	case ReservationConfirmed:
		return inventoryv1.ReservationStatus_RESERVATION_STATUS_CONFIRMED
	case ReservationReleased:
		return inventoryv1.ReservationStatus_RESERVATION_STATUS_RELEASED
	default:
		return inventoryv1.ReservationStatus_RESERVATION_STATUS_UNSPECIFIED
	}
}
