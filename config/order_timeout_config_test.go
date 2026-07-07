package config

import (
	"errors"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestRabbitMQAndOrderTimeoutConfigValidate(t *testing.T) {
	rabbit := validRabbitMQConfig()
	if err := rabbit.Validate(); err != nil {
		t.Fatalf("rabbit config: %v", err)
	}

	t.Run("URL is required", func(t *testing.T) {
		candidate := rabbit
		candidate.URL = "   "
		if err := candidate.Validate(); !errors.Is(err, ErrInvalidRabbitMQURL) {
			t.Fatalf("empty RabbitMQ URL error=%v", err)
		}
	})

	t.Run("order timeout errors propagate through RabbitMQ", func(t *testing.T) {
		candidate := rabbit
		candidate.OrderTimeout.Delay = 0
		if err := candidate.Validate(); !errors.Is(err, ErrInvalidOrderTimeoutDelay) {
			t.Fatalf("zero delay error=%v", err)
		}
	})
}

func TestOrderTimeoutConfigAllowsShortTestDelay(t *testing.T) {
	orderTimeout := validOrderTimeoutConfig()
	orderTimeout.Delay = time.Millisecond
	if err := orderTimeout.Validate(); err != nil {
		t.Fatalf("short test delay must be valid: %v", err)
	}
}

func TestOrderTimeoutConfigRejectsNonPositiveDelay(t *testing.T) {
	for _, delay := range []time.Duration{0, -time.Millisecond} {
		orderTimeout := validOrderTimeoutConfig()
		orderTimeout.Delay = delay
		if err := orderTimeout.Validate(); !errors.Is(err, ErrInvalidOrderTimeoutDelay) {
			t.Fatalf("delay=%s error=%v", delay, err)
		}
	}
}

func TestApplyRabbitMQEnvOverridesUsesNestedOrderTimeout(t *testing.T) {
	t.Setenv("RABBITMQ_URL", "amqps://rabbit.example.test:5671/")
	t.Setenv("ORDER_TIMEOUT_DELAY", "25ms")

	rabbit := validRabbitMQConfig()
	if err := applyRabbitMQEnvOverrides(&rabbit); err != nil {
		t.Fatalf("apply RabbitMQ environment: %v", err)
	}
	if rabbit.URL != "amqps://rabbit.example.test:5671/" {
		t.Fatalf("URL=%q", rabbit.URL)
	}
	if rabbit.OrderTimeout.Delay != 25*time.Millisecond {
		t.Fatalf("delay=%s", rabbit.OrderTimeout.Delay)
	}
}

func TestRabbitMQOrderTimeoutYAMLLayering(t *testing.T) {
	var cfg Config
	err := yaml.Unmarshal([]byte(`
rabbitmq:
  url: amqp://guest:guest@localhost:5672/
  connectTimeout: 1s
  reconnectDelay: 1s
  orderTimeout:
    delay: 15ms
    outboxPollInterval: 10ms
    outboxRetryDelay: 20ms
    publishBatchSize: 5
    consumerPrefetch: 2
`), &cfg)
	if err != nil {
		t.Fatalf("parse layered RabbitMQ config: %v", err)
	}
	if cfg.RabbitMQ.OrderTimeout.Delay != 15*time.Millisecond {
		t.Fatalf("nested delay=%s", cfg.RabbitMQ.OrderTimeout.Delay)
	}
	if err := cfg.RabbitMQ.Validate(); err != nil {
		t.Fatalf("validate layered RabbitMQ config: %v", err)
	}
}

func TestLoadProjectConfigUsesNestedRabbitMQOrderTimeout(t *testing.T) {
	for _, name := range []string{
		"RABBITMQ_URL",
		"ORDER_TIMEOUT_DELAY",
	} {
		t.Setenv(name, "")
	}

	cfg, err := LoadConfig("../config.yml")
	if err != nil {
		t.Fatalf("load project config: %v", err)
	}
	if cfg.RabbitMQ.OrderTimeout.Delay != 30*time.Minute {
		t.Fatalf("project order timeout delay=%s", cfg.RabbitMQ.OrderTimeout.Delay)
	}
}

func validRabbitMQConfig() RabbitMQConfig {
	return RabbitMQConfig{
		URL:            "amqp://guest:guest@localhost:5672/",
		ConnectTimeout: time.Second,
		ReconnectDelay: time.Second,
		OrderTimeout:   validOrderTimeoutConfig(),
	}
}

func validOrderTimeoutConfig() OrderTimeoutConfig {
	return OrderTimeoutConfig{
		Delay:              30 * time.Minute,
		OutboxPollInterval: time.Second,
		OutboxRetryDelay:   5 * time.Second,
		PublishBatchSize:   100,
		ConsumerPrefetch:   10,
	}
}
