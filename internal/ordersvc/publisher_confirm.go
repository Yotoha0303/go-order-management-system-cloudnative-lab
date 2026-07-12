package ordersvc

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	platformmetrics "go-order-management-system/internal/platform/metrics"

	amqp "github.com/rabbitmq/amqp091-go"
)

var (
	errPublisherNacked         = errors.New("rabbitmq publish negatively acknowledged")
	errPublisherConfirmClosed  = errors.New("rabbitmq publisher confirmation channel closed")
	errPublisherConfirmTimeout = errors.New("rabbitmq publisher confirmation timed out")
)

var rabbitPublishDurationBuckets = []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5}

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
	started := time.Now()
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
		recordRabbitPublish("publish_error", time.Since(started))
		return fmt.Errorf("publish rabbitmq message: %w", err)
	}
	if err := waitForPublisherConfirmation(publishCtx, p.confirmations); err != nil {
		recordRabbitPublish(publisherOutcome(err), time.Since(started))
		return err
	}
	recordRabbitPublish("ack", time.Since(started))
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

func publisherOutcome(err error) string {
	switch {
	case errors.Is(err, errPublisherNacked):
		return "nack"
	case errors.Is(err, errPublisherConfirmClosed):
		return "channel_closed"
	case errors.Is(err, errPublisherConfirmTimeout):
		return "timeout"
	default:
		return "other_error"
	}
}

func recordRabbitPublish(outcome string, duration time.Duration) {
	labels := platformmetrics.Labels{"outcome": outcome}
	platformmetrics.Default.IncCounter(
		"go_order_rabbitmq_publish_total",
		"Total RabbitMQ timeout event publish attempts by confirm outcome.",
		labels,
	)
	platformmetrics.Default.ObserveHistogram(
		"go_order_rabbitmq_publish_duration_seconds",
		"RabbitMQ timeout event publish and confirmation duration in seconds by outcome.",
		labels,
		duration.Seconds(),
		rabbitPublishDurationBuckets,
	)
}
