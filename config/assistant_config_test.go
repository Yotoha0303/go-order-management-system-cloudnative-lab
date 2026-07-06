package config

import (
	"errors"
	"testing"
	"time"
)

func TestAssistantConfigValidate(t *testing.T) {
	valid := AssistantConfig{
		Timeout: 8 * time.Second,
		LLM: AssistantLLMConfig{
			Mode:             "mock",
			Provider:         "mock",
			MaxResponseBytes: 1 << 20,
		},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("mock config: %v", err)
	}

	chat := valid
	chat.LLM = AssistantLLMConfig{
		Mode:             "chat_completions",
		Provider:         "provider",
		Endpoint:         "https://llm.example.test/v1/chat/completions",
		Model:            "model",
		APIKey:           "secret",
		MaxResponseBytes: 1 << 20,
	}
	if err := chat.Validate(); err != nil {
		t.Fatalf("chat completions config: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*AssistantConfig)
		want   error
	}{
		{name: "timeout", mutate: func(c *AssistantConfig) { c.Timeout = 0 }, want: ErrInvalidAssistantTimeout},
		{name: "mode", mutate: func(c *AssistantConfig) { c.LLM.Mode = "agent" }, want: ErrInvalidAssistantLLMMode},
		{name: "size", mutate: func(c *AssistantConfig) { c.LLM.MaxResponseBytes = 10 }, want: ErrInvalidAssistantLLMMaxResponse},
		{name: "provider", mutate: func(c *AssistantConfig) { c.LLM.Provider = "" }, want: ErrInvalidAssistantLLMProvider},
		{name: "endpoint", mutate: func(c *AssistantConfig) { c.LLM.Endpoint = "relative" }, want: ErrInvalidAssistantLLMEndpoint},
		{name: "model", mutate: func(c *AssistantConfig) { c.LLM.Model = "" }, want: ErrInvalidAssistantLLMModel},
		{name: "api key", mutate: func(c *AssistantConfig) { c.LLM.APIKey = "" }, want: ErrInvalidAssistantLLMAPIKey},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := chat
			test.mutate(&candidate)
			if err := candidate.Validate(); !errors.Is(err, test.want) {
				t.Fatalf("error=%v want=%v", err, test.want)
			}
		})
	}
}

func TestAssistantTimeoutMustBeLessThanHTTPTimeout(t *testing.T) {
	if err := validateAssistantHTTPTimeout(4*time.Second, 5*time.Second); err != nil {
		t.Fatalf("valid timeout relationship: %v", err)
	}
	if err := validateAssistantHTTPTimeout(5*time.Second, 5*time.Second); !errors.Is(err, ErrAssistantTimeoutExceedsHTTP) {
		t.Fatalf("error=%v want=%v", err, ErrAssistantTimeoutExceedsHTTP)
	}
}
