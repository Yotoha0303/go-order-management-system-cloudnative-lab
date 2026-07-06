package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Server       ServerConfig       `yaml:"server"`
	MySQL        MySQLConfig        `yaml:"mysql"`
	Redis        RedisConfig        `yaml:"redis"`
	RabbitMQ     RabbitMQConfig     `yaml:"rabbitmq"`
	OrderTimeout OrderTimeoutConfig `yaml:"orderTimeout"`
	JWT          JWTConfig          `yaml:"jwt"`
	HttpServer   HttpServer         `yaml:"http"`
	Assistant    AssistantConfig    `yaml:"assistant"`
}

type ServerConfig struct {
	Port int `yaml:"port"`
}

type MySQLConfig struct {
	User     string `yaml:"user"`
	Host     string `yaml:"host"`
	Port     string `yaml:"port"`
	Database string `yaml:"database"`

	MaxOpenConns    int           `yaml:"maxOpenConns"`
	MaxIdleConns    int           `yaml:"maxIdleConns"`
	ConnMaxLifetime time.Duration `yaml:"connMaxLifeTime"`
	ConnMaxIdleTime time.Duration `yaml:"connMaxIdleTime"`
	PingTimeout     time.Duration `yaml:"pingTimeout"`
}

type RedisConfig struct {
	Addr string `yaml:"addr"`
	DB   int    `yaml:"db"`
}

type RabbitMQConfig struct {
	URL            string        `yaml:"url"`
	ConnectTimeout time.Duration `yaml:"connectTimeout"`
	ReconnectDelay time.Duration `yaml:"reconnectDelay"`
}

type OrderTimeoutConfig struct {
	Delay              time.Duration `yaml:"delay"`
	OutboxPollInterval time.Duration `yaml:"outboxPollInterval"`
	OutboxRetryDelay   time.Duration `yaml:"outboxRetryDelay"`
	PublishBatchSize   int           `yaml:"publishBatchSize"`
	ConsumerPrefetch   int           `yaml:"consumerPrefetch"`
}

type JWTConfig struct {
	ExpireHours int `yaml:"expireHours"`
}

type HttpServer struct {
	Server HttpServerConfig `yaml:"server"`
}

type HttpServerConfig struct {
	ReadTimeOut       time.Duration `yaml:"readTimeout"`
	WriteTimeout      time.Duration `yaml:"writeTimeout"`
	IdleTimeout       time.Duration `yaml:"idleTimeout"`
	ReadHeaderTimeout time.Duration `yaml:"readHeaderTimeout"`
	MaxHeaderBytesKib int           `yaml:"maxHeaderBytesKib"`
	Timeout           time.Duration `yaml:"timeout"`
}

type AssistantConfig struct {
	Timeout time.Duration      `yaml:"timeout"`
	LLM     AssistantLLMConfig `yaml:"llm"`
}

type AssistantLLMConfig struct {
	Mode             string `yaml:"mode"`
	Provider         string `yaml:"provider"`
	Endpoint         string `yaml:"endpoint"`
	Model            string `yaml:"model"`
	MaxResponseBytes int64  `yaml:"maxResponseBytes"`
	APIKey           string `yaml:"-"`
}

func LoadEnv() {
	_ = godotenv.Load()
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file %s failed: %w", path, err)
	}

	var cfg Config

	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		return nil, fmt.Errorf("parse config file %s failed: %w", path, err)
	}

	if err := applyEnvOverrides(&cfg); err != nil {
		return nil, err
	}

	if err = cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func applyEnvOverrides(cfg *Config) error {
	if v := os.Getenv("APP_PORT"); v != "" {
		port, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("invalid APP_PORT: %w", err)
		}
		cfg.Server.Port = port
	}

	if v := os.Getenv("DB_HOST"); v != "" {
		cfg.MySQL.Host = v
	}
	if v := os.Getenv("DB_PORT"); v != "" {
		cfg.MySQL.Port = v
	}
	if v := os.Getenv("DB_USER"); v != "" {
		cfg.MySQL.User = v
	}
	if v := os.Getenv("DB_NAME"); v != "" {
		cfg.MySQL.Database = v
	}

	if v := os.Getenv("REDIS_ADDR"); v != "" {
		cfg.Redis.Addr = v
	}
	if v := os.Getenv("REDIS_DB"); v != "" {
		db, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("invalid REDIS_DB: %w", err)
		}
		cfg.Redis.DB = db
	}
	if v := os.Getenv("RABBITMQ_URL"); v != "" {
		cfg.RabbitMQ.URL = v
	}
	if v := os.Getenv("ORDER_TIMEOUT_DELAY"); v != "" {
		delay, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("invalid ORDER_TIMEOUT_DELAY: %w", err)
		}
		cfg.OrderTimeout.Delay = delay
	}

	if v := os.Getenv("JWT_EXPIRE_HOURS"); v != "" {
		hours, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("invalid JWT_EXPIRE_HOURS: %w", err)
		}
		cfg.JWT.ExpireHours = hours
	}

	if v := os.Getenv("ASSISTANT_TIMEOUT"); v != "" {
		timeout, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("invalid ASSISTANT_TIMEOUT: %w", err)
		}
		cfg.Assistant.Timeout = timeout
	}
	if v := os.Getenv("LLM_MODE"); v != "" {
		cfg.Assistant.LLM.Mode = v
	}
	if v := os.Getenv("LLM_PROVIDER"); v != "" {
		cfg.Assistant.LLM.Provider = v
	}
	if v := os.Getenv("LLM_ENDPOINT"); v != "" {
		cfg.Assistant.LLM.Endpoint = v
	}
	if v := os.Getenv("LLM_MODEL"); v != "" {
		cfg.Assistant.LLM.Model = v
	}
	if v := os.Getenv("LLM_MAX_RESPONSE_BYTES"); v != "" {
		maxBytes, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid LLM_MAX_RESPONSE_BYTES: %w", err)
		}
		cfg.Assistant.LLM.MaxResponseBytes = maxBytes
	}
	cfg.Assistant.LLM.APIKey = os.Getenv("LLM_API_KEY")

	return nil
}
