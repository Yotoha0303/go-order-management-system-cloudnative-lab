package assistantadapter

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go-order-management-system/internal/assistant"
)

func TestChatCompletionsClientParsesIntentAndUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-api-key" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		var request chatCompletionsRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if request.Model != "configured-model" || len(request.Messages) != 2 {
			t.Errorf("request = %+v", request)
		}
		if request.Messages[0].Content != assistant.SystemPrompt() || request.Messages[1].Content != "低库存商品" {
			t.Errorf("messages = %+v", request.Messages)
		}
		if request.ResponseFormat.Type != "json_object" || request.Temperature != 0 {
			t.Errorf("response controls = %+v, temperature=%f", request.ResponseFormat, request.Temperature)
		}
		_, _ = io.WriteString(w, `{
			"model":"actual-model",
			"choices":[{"message":{"content":"{\"intent\":\"get_low_stock_products\",\"arguments\":{\"threshold\":5}}"}}],
			"usage":{"prompt_tokens":20,"completion_tokens":8,"total_tokens":28}
		}`)
	}))
	defer server.Close()

	client := newTestChatCompletionsClient(t, server.URL, 1<<20)
	result, usage, err := client.ParseIntent(context.Background(), "低库存商品")
	if err != nil {
		t.Fatalf("ParseIntent: %v", err)
	}
	if result.Intent != assistant.IntentGetLowStockProducts || usage.Model != "actual-model" || usage.TotalTokens != 28 {
		t.Fatalf("result=%+v usage=%+v", result, usage)
	}
}

func TestChatCompletionsClientDisablesThinkingForDeepSeek(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request chatCompletionsRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if request.Thinking == nil || request.Thinking.Type != "disabled" {
			t.Errorf("thinking = %+v, want disabled", request.Thinking)
		}
		_, _ = io.WriteString(w, `{"model":"deepseek-v4-flash","choices":[{"message":{"content":"{\"intent\":\"get_low_stock_products\",\"arguments\":{}}"}}]}`)
	}))
	defer server.Close()

	client := newTestChatCompletionsClientWithProvider(t, server.URL, "deepseek", 1<<20)
	if _, _, err := client.ParseIntent(context.Background(), "低库存商品"); err != nil {
		t.Fatalf("ParseIntent: %v", err)
	}
}

func TestChatCompletionsClientClassifiesProviderFailures(t *testing.T) {
	tests := []struct {
		name string
		body string
		code assistant.ErrorCode
	}{
		{name: "invalid JSON", body: `{`, code: assistant.CodeInvalidModelResponse},
		{name: "no choices", body: `{"choices":[]}`, code: assistant.CodeInvalidModelResponse},
		{name: "empty content", body: `{"choices":[{"message":{"content":""}}]}`, code: assistant.CodeInvalidModelResponse},
		{name: "unknown intent", body: `{"choices":[{"message":{"content":"{\"intent\":\"delete_order\",\"arguments\":{}}"}}]}`, code: assistant.CodeUnknownIntent},
		{name: "negative usage", body: `{"choices":[{"message":{"content":"{\"intent\":\"get_low_stock_products\",\"arguments\":{}}"}}],"usage":{"total_tokens":-1}}`, code: assistant.CodeInvalidModelResponse},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, tt.body)
			}))
			defer server.Close()
			client := newTestChatCompletionsClient(t, server.URL, 1<<20)
			_, _, err := client.ParseIntent(context.Background(), "message")
			if !assistant.IsCode(err, tt.code) {
				t.Fatalf("ParseIntent error = %v, want %s", err, tt.code)
			}
		})
	}
}

func TestChatCompletionsClientRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, strings.Repeat("x", 1025))
	}))
	defer server.Close()

	client := newTestChatCompletionsClient(t, server.URL, 1024)
	_, _, err := client.ParseIntent(context.Background(), "message")
	if !assistant.IsCode(err, assistant.CodeInvalidModelResponse) {
		t.Fatalf("ParseIntent error = %v, want %s", err, assistant.CodeInvalidModelResponse)
	}
}

func TestChatCompletionsClientDoesNotExposeProviderBodyOrAPIKey(t *testing.T) {
	secret := "test-api-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, "provider body contains "+secret)
	}))
	defer server.Close()

	client := newTestChatCompletionsClient(t, server.URL, 1<<20)
	_, _, err := client.ParseIntent(context.Background(), "message")
	if !assistant.IsCode(err, assistant.CodeLLMUnavailable) {
		t.Fatalf("ParseIntent error = %v", err)
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "provider body") {
		t.Fatalf("error exposed provider data: %v", err)
	}
}

func TestChatCompletionsClientHonorsCanceledContext(t *testing.T) {
	client := newTestChatCompletionsClient(t, "http://127.0.0.1:1/chat", 1<<20)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := client.ParseIntent(ctx, "message")
	if !assistant.IsCode(err, assistant.CodeRequestTimeout) {
		t.Fatalf("ParseIntent error = %v", err)
	}
}

func TestNewChatCompletionsClientRejectsInvalidConfig(t *testing.T) {
	base := ChatCompletionsConfig{
		Endpoint:         "https://example.test/chat",
		APIKey:           "secret",
		Model:            "model",
		Provider:         "provider",
		MaxResponseBytes: 1024,
		HTTPClient:       &http.Client{Timeout: time.Second},
	}
	tests := []struct {
		name   string
		mutate func(*ChatCompletionsConfig)
	}{
		{name: "endpoint", mutate: func(c *ChatCompletionsConfig) { c.Endpoint = "relative" }},
		{name: "api key", mutate: func(c *ChatCompletionsConfig) { c.APIKey = "" }},
		{name: "model", mutate: func(c *ChatCompletionsConfig) { c.Model = "" }},
		{name: "provider", mutate: func(c *ChatCompletionsConfig) { c.Provider = "" }},
		{name: "size", mutate: func(c *ChatCompletionsConfig) { c.MaxResponseBytes = 0 }},
		{name: "HTTP client", mutate: func(c *ChatCompletionsConfig) { c.HTTPClient = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := base
			tt.mutate(&config)
			if _, err := NewChatCompletionsClient(config); err == nil {
				t.Fatal("NewChatCompletionsClient: want error")
			}
		})
	}
}

func newTestChatCompletionsClient(t *testing.T, endpoint string, maxResponseBytes int64) *ChatCompletionsClient {
	t.Helper()
	return newTestChatCompletionsClientWithProvider(t, endpoint, "test-provider", maxResponseBytes)
}

func newTestChatCompletionsClientWithProvider(t *testing.T, endpoint, provider string, maxResponseBytes int64) *ChatCompletionsClient {
	t.Helper()
	client, err := NewChatCompletionsClient(ChatCompletionsConfig{
		Endpoint:         endpoint,
		APIKey:           "test-api-key",
		Model:            "configured-model",
		Provider:         provider,
		MaxResponseBytes: maxResponseBytes,
		HTTPClient:       &http.Client{Timeout: time.Second},
	})
	if err != nil {
		t.Fatalf("NewChatCompletionsClient: %v", err)
	}
	return client
}
