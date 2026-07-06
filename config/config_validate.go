package config

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidServerPort                  = errors.New("invalid server port")
	ErrInvalidMySQLPort                   = errors.New("invalid mysql port")
	ErrMySQLDatabaseNotFound              = errors.New("MySQL database name not found")
	ErrMySQLUserNotFound                  = errors.New("MySQL user not found")
	ErrMySQLHostNotFound                  = errors.New("MySQL host not found")
	ErrInvalidHttpServerReadTimeout       = errors.New("invalid server read time out")
	ErrInvalidHttpServerWriteTimeout      = errors.New("invalid server write time out")
	ErrInvalidHttpServerIdleTimeout       = errors.New("invalid server idle time out")
	ErrInvalidHttpServerReadHeaderTimeout = errors.New("invalid server read header time out")
	ErrInvalidHttpServerMaxHeaderBytes    = errors.New("invalid server max header bytes")
	ErrInvalidHttpServerTimeout           = errors.New("invalid http server time out")
	ErrMySQLMaxOpenConnsFailed            = errors.New("MySQL max open conns failed")
	ErrMySQLMaxIdleConnsFailed            = errors.New("MySQL mysql max idle conns failed")
	ErrMySQLInvalidConnMaxIdleTime        = errors.New("invalid mysql conn max idle time")
	ErrMySQLInvalidConnMaxLifetime        = errors.New("invalid mysql conn max life time")
	ErrMySQLInvalidPingTimeout            = errors.New("invalid mysql conn ping time out")
	ErrInvalidJWTExpireHours              = errors.New("invalid jwt expire hours")
	ErrInvalidAssistantTimeout            = errors.New("invalid assistant timeout")
	ErrInvalidAssistantLLMMode            = errors.New("invalid assistant llm mode")
	ErrInvalidAssistantLLMProvider        = errors.New("invalid assistant llm provider")
	ErrInvalidAssistantLLMEndpoint        = errors.New("invalid assistant llm endpoint")
	ErrInvalidAssistantLLMModel           = errors.New("invalid assistant llm model")
	ErrInvalidAssistantLLMAPIKey          = errors.New("invalid assistant llm api key")
	ErrInvalidAssistantLLMMaxResponse     = errors.New("invalid assistant llm max response bytes")
	ErrAssistantTimeoutExceedsHTTP        = errors.New("assistant timeout must be less than http timeout")
	ErrInvalidRabbitMQURL                 = errors.New("invalid rabbitmq url")
	ErrInvalidRabbitMQConnectTimeout      = errors.New("invalid rabbitmq connect timeout")
	ErrInvalidRabbitMQReconnectDelay      = errors.New("invalid rabbitmq reconnect delay")
	ErrInvalidOrderTimeoutDelay           = errors.New("invalid order timeout delay")
	ErrInvalidOrderTimeoutPollInterval    = errors.New("invalid order timeout outbox poll interval")
	ErrInvalidOrderTimeoutRetryDelay      = errors.New("invalid order timeout outbox retry delay")
	ErrInvalidOrderTimeoutBatchSize       = errors.New("invalid order timeout publish batch size")
	ErrInvalidOrderTimeoutPrefetch        = errors.New("invalid order timeout consumer prefetch")
)

func (c Config) Validate() error {
	server := c.Server
	mysql := c.MySQL
	http := c.HttpServer.Server
	jwt := c.JWT

	if server.Port <= 0 {
		return ErrInvalidServerPort
	}

	if mysql.Host == "" {
		return ErrMySQLHostNotFound
	}

	mysqlPort, err := strconv.Atoi(mysql.Port)
	if err != nil || mysqlPort <= 0 || mysqlPort > 65535 {
		return ErrInvalidMySQLPort
	}

	if mysql.Database == "" {
		return ErrMySQLDatabaseNotFound
	}

	if mysql.User == "" {
		return ErrMySQLUserNotFound
	}

	if http.ReadTimeOut <= 0 {
		return ErrInvalidHttpServerReadTimeout
	}

	if http.WriteTimeout <= 0 {
		return ErrInvalidHttpServerWriteTimeout
	}

	if http.IdleTimeout <= 0 {
		return ErrInvalidHttpServerIdleTimeout
	}

	if http.ReadHeaderTimeout <= 0 {
		return ErrInvalidHttpServerReadHeaderTimeout
	}

	if http.MaxHeaderBytesKib <= 0 {
		return ErrInvalidHttpServerMaxHeaderBytes
	}

	if http.Timeout <= 0 {
		return ErrInvalidHttpServerTimeout
	}

	if mysql.MaxOpenConns <= 0 {
		return ErrMySQLMaxOpenConnsFailed
	}

	if mysql.MaxIdleConns < 0 || mysql.MaxIdleConns > mysql.MaxOpenConns {
		return ErrMySQLMaxIdleConnsFailed
	}

	if mysql.ConnMaxIdleTime <= 0 {
		return ErrMySQLInvalidConnMaxIdleTime
	}

	if mysql.ConnMaxLifetime <= 0 {
		return ErrMySQLInvalidConnMaxLifetime
	}

	if mysql.PingTimeout <= 0 {
		return ErrMySQLInvalidPingTimeout
	}

	if jwt.ExpireHours <= 0 {
		return ErrInvalidJWTExpireHours
	}

	if err := c.RabbitMQ.Validate(); err != nil {
		return err
	}
	if err := c.OrderTimeout.Validate(); err != nil {
		return err
	}

	if err := c.Assistant.Validate(); err != nil {
		return err
	}
	if err := validateAssistantHTTPTimeout(c.Assistant.Timeout, http.Timeout); err != nil {
		return err
	}

	return nil
}

func (c RabbitMQConfig) Validate() error {
	u, err := url.ParseRequestURI(strings.TrimSpace(c.URL))
	if err != nil || u.Host == "" || (u.Scheme != "amqp" && u.Scheme != "amqps") {
		return ErrInvalidRabbitMQURL
	}
	if c.ConnectTimeout <= 0 || c.ConnectTimeout > time.Minute {
		return ErrInvalidRabbitMQConnectTimeout
	}
	if c.ReconnectDelay <= 0 || c.ReconnectDelay > time.Minute {
		return ErrInvalidRabbitMQReconnectDelay
	}
	return nil
}

func (c OrderTimeoutConfig) Validate() error {
	if c.Delay <= 0 || c.Delay > 24*time.Hour {
		return ErrInvalidOrderTimeoutDelay
	}
	if c.OutboxPollInterval <= 0 || c.OutboxPollInterval > time.Minute {
		return ErrInvalidOrderTimeoutPollInterval
	}
	if c.OutboxRetryDelay <= 0 || c.OutboxRetryDelay > time.Hour {
		return ErrInvalidOrderTimeoutRetryDelay
	}
	if c.PublishBatchSize < 1 || c.PublishBatchSize > 1000 {
		return ErrInvalidOrderTimeoutBatchSize
	}
	if c.ConsumerPrefetch < 1 || c.ConsumerPrefetch > 1000 {
		return ErrInvalidOrderTimeoutPrefetch
	}
	return nil
}

func validateAssistantHTTPTimeout(assistantTimeout, httpTimeout time.Duration) error {
	if assistantTimeout >= httpTimeout {
		return ErrAssistantTimeoutExceedsHTTP
	}
	return nil
}

func (c AssistantConfig) Validate() error {
	if c.Timeout <= 0 || c.Timeout > time.Minute {
		return ErrInvalidAssistantTimeout
	}
	if c.LLM.MaxResponseBytes < 1<<10 || c.LLM.MaxResponseBytes > 4<<20 {
		return ErrInvalidAssistantLLMMaxResponse
	}

	mode := strings.TrimSpace(c.LLM.Mode)
	switch mode {
	case "mock":
		return nil
	case "chat_completions":
		if strings.TrimSpace(c.LLM.Provider) == "" {
			return ErrInvalidAssistantLLMProvider
		}
		endpoint, err := url.ParseRequestURI(strings.TrimSpace(c.LLM.Endpoint))
		if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
			return ErrInvalidAssistantLLMEndpoint
		}
		if strings.TrimSpace(c.LLM.Model) == "" {
			return ErrInvalidAssistantLLMModel
		}
		if strings.TrimSpace(c.LLM.APIKey) == "" {
			return ErrInvalidAssistantLLMAPIKey
		}
		return nil
	default:
		return ErrInvalidAssistantLLMMode
	}
}
