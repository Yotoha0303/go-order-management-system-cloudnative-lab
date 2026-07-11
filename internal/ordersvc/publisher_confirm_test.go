package ordersvc

import (
	"context"
	"errors"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

func TestWaitForPublisherConfirmation(t *testing.T) {
	t.Run("ack", func(t *testing.T) {
		confirmations := make(chan amqp.Confirmation, 1)
		confirmations <- amqp.Confirmation{DeliveryTag: 1, Ack: true}

		if err := waitForPublisherConfirmation(context.Background(), confirmations); err != nil {
			t.Fatalf("wait for ack: %v", err)
		}
	})

	t.Run("nack", func(t *testing.T) {
		confirmations := make(chan amqp.Confirmation, 1)
		confirmations <- amqp.Confirmation{DeliveryTag: 2, Ack: false}

		err := waitForPublisherConfirmation(context.Background(), confirmations)
		if !errors.Is(err, errPublisherNacked) {
			t.Fatalf("expected nack error, got %v", err)
		}
	})

	t.Run("closed", func(t *testing.T) {
		confirmations := make(chan amqp.Confirmation)
		close(confirmations)

		err := waitForPublisherConfirmation(context.Background(), confirmations)
		if !errors.Is(err, errPublisherConfirmClosed) {
			t.Fatalf("expected closed error, got %v", err)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		confirmations := make(chan amqp.Confirmation)
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		defer cancel()

		err := waitForPublisherConfirmation(ctx, confirmations)
		if !errors.Is(err, errPublisherConfirmTimed) {
			t.Fatalf("expected timeout error, got %v", err)
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected deadline cause, got %v", err)
		}
	})
}
