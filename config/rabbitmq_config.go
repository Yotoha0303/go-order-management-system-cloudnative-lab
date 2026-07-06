package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	maxRabbitMQConnectTimeout   = time.Minute
	maxRabbitMQReconnectDelay   = time.Minute
	maxOrderTimeoutDelay        = 24 * time.Hour
	maxOutboxPollInterval       = time.Minute
	maxOutboxRetryDelay         = time.Hour
	maxOrderTimeoutBatchSize    = 1000
	maxOrderTimeoutPrefetchSize = 1000
)

var (
	ErrInvalidRabbitMQURL              = errors.New("invalid rabbitmq url")
	ErrInvalidRabbitMQConnectTimeout   = errors.New("invalid rabbitmq connect timeout")
	ErrInvalidRabbitMQReconnectDelay   = errors.New("invalid rabbitmq reconnect delay")
	ErrInvalidOrderTimeoutDelay        = errors.New("invalid order timeout delay")
	ErrInvalidOrderTimeoutPollInterval = errors.New("invalid order timeout outbox poll interval")
	ErrInvalidOrderTimeoutRetryDelay   = errors.New("invalid order timeout outbox retry delay")
	ErrInvalidOrderTimeoutBatchSize    = errors.New("invalid order timeout publish batch size")
	ErrInvalidOrderTimeoutPrefetch     = errors.New("invalid order timeout consumer prefetch")
)

type RabbitMQConfig struct {
	URL            string             `yaml:"url"`
	ConnectTimeout time.Duration      `yaml:"connectTimeout"`
	ReconnectDelay time.Duration      `yaml:"reconnectDelay"`
	OrderTimeout   OrderTimeoutConfig `yaml:"orderTimeout"`
}

type OrderTimeoutConfig struct {
	Delay              time.Duration `yaml:"delay"`
	OutboxPollInterval time.Duration `yaml:"outboxPollInterval"`
	OutboxRetryDelay   time.Duration `yaml:"outboxRetryDelay"`
	PublishBatchSize   int           `yaml:"publishBatchSize"`
	ConsumerPrefetch   int           `yaml:"consumerPrefetch"`
}

func (c RabbitMQConfig) Validate() error {
	rabbitURL := strings.TrimSpace(c.URL)
	if rabbitURL == "" {
		return ErrInvalidRabbitMQURL
	}
	u, err := url.ParseRequestURI(rabbitURL)
	if err != nil || u.Host == "" || (u.Scheme != "amqp" && u.Scheme != "amqps") {
		return ErrInvalidRabbitMQURL
	}
	if c.ConnectTimeout <= 0 || c.ConnectTimeout > maxRabbitMQConnectTimeout {
		return ErrInvalidRabbitMQConnectTimeout
	}
	if c.ReconnectDelay <= 0 || c.ReconnectDelay > maxRabbitMQReconnectDelay {
		return ErrInvalidRabbitMQReconnectDelay
	}
	return c.OrderTimeout.Validate()
}

func (c OrderTimeoutConfig) Validate() error {
	// Do not impose a production-sized minimum: integration tests need millisecond delays.
	if c.Delay <= 0 || c.Delay > maxOrderTimeoutDelay {
		return ErrInvalidOrderTimeoutDelay
	}
	if c.OutboxPollInterval <= 0 || c.OutboxPollInterval > maxOutboxPollInterval {
		return ErrInvalidOrderTimeoutPollInterval
	}
	if c.OutboxRetryDelay <= 0 || c.OutboxRetryDelay > maxOutboxRetryDelay {
		return ErrInvalidOrderTimeoutRetryDelay
	}
	if c.PublishBatchSize < 1 || c.PublishBatchSize > maxOrderTimeoutBatchSize {
		return ErrInvalidOrderTimeoutBatchSize
	}
	if c.ConsumerPrefetch < 1 || c.ConsumerPrefetch > maxOrderTimeoutPrefetchSize {
		return ErrInvalidOrderTimeoutPrefetch
	}
	return nil
}

func applyRabbitMQEnvOverrides(cfg *RabbitMQConfig) error {
	if v := os.Getenv("RABBITMQ_URL"); v != "" {
		cfg.URL = v
	}
	if v := os.Getenv("ORDER_TIMEOUT_DELAY"); v != "" {
		delay, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("invalid ORDER_TIMEOUT_DELAY: %w", err)
		}
		cfg.OrderTimeout.Delay = delay
	}
	return nil
}
