package assistantadapter

import (
	"testing"

	"go-order-management-system/internal/assistant"
	"go-order-management-system/internal/model"
)

func TestNewMySQLRepositoryRejectsNilDB(t *testing.T) {
	if _, err := NewMySQLRepository(nil); err == nil {
		t.Fatal("NewMySQLRepository: want error")
	}
}

func TestMapOrderStatus(t *testing.T) {
	tests := map[int8]assistant.OrderStatus{
		model.OrderStatusPending:   assistant.OrderStatusPending,
		model.OrderStatusPaid:      assistant.OrderStatusPaid,
		model.OrderStatusFinished:  assistant.OrderStatusFinished,
		model.OrderStatusCancelled: assistant.OrderStatusCancelled,
	}
	for source, want := range tests {
		got, err := mapOrderStatus(source)
		if err != nil || got != want {
			t.Fatalf("mapOrderStatus(%d) = %q, %v; want %q", source, got, err, want)
		}
	}
	if _, err := mapOrderStatus(99); err == nil {
		t.Fatal("mapOrderStatus(99): want error")
	}
}
