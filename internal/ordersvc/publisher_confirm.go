package ordersvc

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

var (
	errPublisherNacked         = errors.New("rabbitmq publish negatively acknowledged")
	errPublisherConfirmClosed  = errors.New("rabbitmq publisher confirmation channel closed")
	errPublisherConfirmTimeout = errors.New("rabbitmq publisher confirmation timed out")
)

type timeoutEventPublisher interface {
	Publish(ctx context.Context, event TimeoutOutbox, payload []byte, delay time.Duration) error
}

type confirmedAMQPPublisher struct {
	channel        *amqp.Channel
	confirmations  <-chan amqp.Confirmation
	exchange       string
	routingKey     string
	confirmTimeout time.Duration
}

func newConfirmedAMQPPublisher(channel *amqp.Channel, exchange, routingKey string, confirmTimeout time.Duration) (*confirmedAMQPPublisher, error) {
	if channel == nil {
		return nil, errors.New("rabbitmq publisher channel is required")
	}
	if confirmTimeout <= 0 {
		confirmTimeout = 5 * time.Second
	}
	if err := channel.Confirm(false); err != nil {
		return nil, fmt.Errorf("enable rabbitmq publisher confirms: %w", err)
	}
	confirmations := channel.NotifyPublish(make(chan amqp.Confirmation, 1))
	return &confirmedAMQPPublisher{
		channel:        channel,
		confirmations:  confirmations,
		exchange:       exchange,
		routingKey:     routingKey,
		confirmTimeout: confirmTimeout,
	}, nil
}

func (p *confirmedAMQPPublisher) Publish(ctx context.Context, event TimeoutOutbox, payload []byte, delay time.Duration) error {
	if delay < time.Second {
		delay = time.Second
	}
	publishCtx, cancel := context.WithTimeout(ctx, p.confirmTimeout)
	defer cancel()

	if err := p.channel.PublishWithContext(publishCtx, p.exchange, p.routingKey, false, false, amqp.Publishing{
		DeliveryMode: amqp.Persistent,
		ContentType:  "application/json",
		MessageId:    strconv.FormatUint(event.ID, 10),
		Expiration:   strconv.FormatInt(delay.Milliseconds(), 10),
		Timestamp:    time.Now(),
		Body:         payload,
	}); err != nil {
		return fmt.Errorf("publish rabbitmq message: %w", err)
	}
	if err := waitForPublisherConfirmation(publishCtx, p.confirmations); err != nil {
		return err
	}
	return nil
}

func waitForPublisherConfirmation(ctx context.Context, confirmations <-chan amqp.Confirmation) error {
	select {
	case confirmation, ok := <-confirmations:
		if !ok {
			return errPublisherConfirmClosed
		}
		if !confirmation.Ack {
			return fmt.Errorf("%w: delivery_tag=%d", errPublisherNacked, confirmation.DeliveryTag)
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("%w: %w", errPublisherConfirmTimeout, ctx.Err())
	}
}
