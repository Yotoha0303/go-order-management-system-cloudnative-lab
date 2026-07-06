package config

import (
	"errors"
	"testing"
	"time"
)

func TestRabbitMQAndOrderTimeoutConfigValidate(t *testing.T) {
	rabbit := RabbitMQConfig{URL: "amqp://guest:guest@localhost:5672/", ConnectTimeout: time.Second, ReconnectDelay: time.Second}
	if err := rabbit.Validate(); err != nil {
		t.Fatalf("rabbit config: %v", err)
	}
	if err := (RabbitMQConfig{}).Validate(); !errors.Is(err, ErrInvalidRabbitMQURL) {
		t.Fatalf("empty RabbitMQ error=%v", err)
	}

	orderTimeout := OrderTimeoutConfig{
		Delay:              30 * time.Minute,
		OutboxPollInterval: time.Second,
		OutboxRetryDelay:   5 * time.Second,
		PublishBatchSize:   100,
		ConsumerPrefetch:   10,
	}
	if err := orderTimeout.Validate(); err != nil {
		t.Fatalf("order timeout config: %v", err)
	}
	orderTimeout.Delay = 0
	if err := orderTimeout.Validate(); !errors.Is(err, ErrInvalidOrderTimeoutDelay) {
		t.Fatalf("zero delay error=%v", err)
	}
}
