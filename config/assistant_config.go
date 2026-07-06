package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	maxAssistantTimeout       = time.Minute
	minAssistantResponseBytes = 1 << 10
	maxAssistantResponseBytes = 4 << 20
)

var (
	ErrInvalidAssistantTimeout        = errors.New("invalid assistant timeout")
	ErrInvalidAssistantLLMMode        = errors.New("invalid assistant llm mode")
	ErrInvalidAssistantLLMProvider    = errors.New("invalid assistant llm provider")
	ErrInvalidAssistantLLMEndpoint    = errors.New("invalid assistant llm endpoint")
	ErrInvalidAssistantLLMModel       = errors.New("invalid assistant llm model")
	ErrInvalidAssistantLLMAPIKey      = errors.New("invalid assistant llm api key")
	ErrInvalidAssistantLLMMaxResponse = errors.New("invalid assistant llm max response bytes")
	ErrAssistantTimeoutExceedsHTTP    = errors.New("assistant timeout must be less than http timeout")
)

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

func (c AssistantConfig) Validate() error {
	if c.Timeout <= 0 || c.Timeout > maxAssistantTimeout {
		return ErrInvalidAssistantTimeout
	}
	return c.LLM.Validate()
}

func (c AssistantLLMConfig) Validate() error {
	if c.MaxResponseBytes < minAssistantResponseBytes || c.MaxResponseBytes > maxAssistantResponseBytes {
		return ErrInvalidAssistantLLMMaxResponse
	}

	switch strings.TrimSpace(c.Mode) {
	case "mock":
		// Mock mode is fully local and does not require endpoint, model, provider, or API key.
		return nil
	case "chat_completions":
		if strings.TrimSpace(c.Provider) == "" {
			return ErrInvalidAssistantLLMProvider
		}
		endpoint, err := url.ParseRequestURI(strings.TrimSpace(c.Endpoint))
		if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
			return ErrInvalidAssistantLLMEndpoint
		}
		if strings.TrimSpace(c.Model) == "" {
			return ErrInvalidAssistantLLMModel
		}
		if strings.TrimSpace(c.APIKey) == "" {
			return ErrInvalidAssistantLLMAPIKey
		}
		return nil
	default:
		return ErrInvalidAssistantLLMMode
	}
}

func validateAssistantHTTPTimeout(assistantTimeout, httpTimeout time.Duration) error {
	if assistantTimeout >= httpTimeout {
		return ErrAssistantTimeoutExceedsHTTP
	}
	return nil
}

func applyAssistantEnvOverrides(cfg *AssistantConfig) error {
	if v := os.Getenv("ASSISTANT_TIMEOUT"); v != "" {
		timeout, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("invalid ASSISTANT_TIMEOUT: %w", err)
		}
		cfg.Timeout = timeout
	}
	if v := os.Getenv("LLM_MODE"); v != "" {
		cfg.LLM.Mode = v
	}
	if v := os.Getenv("LLM_PROVIDER"); v != "" {
		cfg.LLM.Provider = v
	}
	if v := os.Getenv("LLM_ENDPOINT"); v != "" {
		cfg.LLM.Endpoint = v
	}
	if v := os.Getenv("LLM_MODEL"); v != "" {
		cfg.LLM.Model = v
	}
	if v := os.Getenv("LLM_MAX_RESPONSE_BYTES"); v != "" {
		maxBytes, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid LLM_MAX_RESPONSE_BYTES: %w", err)
		}
		cfg.LLM.MaxResponseBytes = maxBytes
	}
	cfg.LLM.APIKey = os.Getenv("LLM_API_KEY")
	return nil
}
