package config

import (
	"errors"
	"testing"
	"time"
)

func TestAssistantConfigValidate(t *testing.T) {
	mock := AssistantConfig{
		Timeout: 8 * time.Second,
		LLM: AssistantLLMConfig{
			Mode:             "mock",
			MaxResponseBytes: 1 << 20,
		},
	}
	if err := mock.Validate(); err != nil {
		t.Fatalf("mock config: %v", err)
	}

	chat := mock
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

	chatTests := []struct {
		name   string
		mutate func(*AssistantConfig)
		want   error
	}{
		{name: "mode", mutate: func(c *AssistantConfig) { c.LLM.Mode = "agent" }, want: ErrInvalidAssistantLLMMode},
		{name: "provider", mutate: func(c *AssistantConfig) { c.LLM.Provider = "" }, want: ErrInvalidAssistantLLMProvider},
		{name: "endpoint empty", mutate: func(c *AssistantConfig) { c.LLM.Endpoint = "" }, want: ErrInvalidAssistantLLMEndpoint},
		{name: "endpoint relative", mutate: func(c *AssistantConfig) { c.LLM.Endpoint = "relative" }, want: ErrInvalidAssistantLLMEndpoint},
		{name: "model", mutate: func(c *AssistantConfig) { c.LLM.Model = "" }, want: ErrInvalidAssistantLLMModel},
		{name: "api key", mutate: func(c *AssistantConfig) { c.LLM.APIKey = "" }, want: ErrInvalidAssistantLLMAPIKey},
	}
	for _, test := range chatTests {
		t.Run(test.name, func(t *testing.T) {
			candidate := chat
			test.mutate(&candidate)
			if err := candidate.Validate(); !errors.Is(err, test.want) {
				t.Fatalf("error=%v want=%v", err, test.want)
			}
		})
	}
}

func TestAssistantConfigTimeoutBounds(t *testing.T) {
	valid := AssistantConfig{
		Timeout: maxAssistantTimeout,
		LLM: AssistantLLMConfig{
			Mode:             "mock",
			MaxResponseBytes: 1 << 20,
		},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("maximum timeout must be valid: %v", err)
	}

	for _, timeout := range []time.Duration{0, -time.Second, maxAssistantTimeout + time.Nanosecond} {
		candidate := valid
		candidate.Timeout = timeout
		if err := candidate.Validate(); !errors.Is(err, ErrInvalidAssistantTimeout) {
			t.Fatalf("timeout=%s error=%v", timeout, err)
		}
	}
}

func TestAssistantLLMMaxResponseBytesBounds(t *testing.T) {
	valid := AssistantLLMConfig{Mode: "mock", MaxResponseBytes: maxAssistantResponseBytes}
	if err := valid.Validate(); err != nil {
		t.Fatalf("maximum response size must be valid: %v", err)
	}

	for _, size := range []int64{minAssistantResponseBytes - 1, maxAssistantResponseBytes + 1} {
		candidate := valid
		candidate.MaxResponseBytes = size
		if err := candidate.Validate(); !errors.Is(err, ErrInvalidAssistantLLMMaxResponse) {
			t.Fatalf("maxResponseBytes=%d error=%v", size, err)
		}
	}
}

func TestAssistantMockModeDoesNotRequireRemoteCredentials(t *testing.T) {
	config := AssistantLLMConfig{
		Mode:             "mock",
		Provider:         "",
		Endpoint:         "",
		Model:            "",
		APIKey:           "",
		MaxResponseBytes: 1 << 20,
	}
	if err := config.Validate(); err != nil {
		t.Fatalf("mock mode must not require remote credentials: %v", err)
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
