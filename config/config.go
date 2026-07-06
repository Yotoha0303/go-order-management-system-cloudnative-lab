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
	Server     ServerConfig    `yaml:"server"`
	MySQL      MySQLConfig     `yaml:"mysql"`
	Redis      RedisConfig     `yaml:"redis"`
	RabbitMQ   RabbitMQConfig  `yaml:"rabbitmq"`
	JWT        JWTConfig       `yaml:"jwt"`
	HttpServer HttpServer      `yaml:"http"`
	Assistant  AssistantConfig `yaml:"assistant"`
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
	if err := applyRabbitMQEnvOverrides(&cfg.RabbitMQ); err != nil {
		return err
	}

	if v := os.Getenv("JWT_EXPIRE_HOURS"); v != "" {
		hours, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("invalid JWT_EXPIRE_HOURS: %w", err)
		}
		cfg.JWT.ExpireHours = hours
	}

	if err := applyAssistantEnvOverrides(&cfg.Assistant); err != nil {
		return err
	}

	return nil
}
