package assistantadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"go-order-management-system/internal/assistant"
)

type ChatCompletionsConfig struct {
	Endpoint         string
	APIKey           string
	Model            string
	Provider         string
	MaxResponseBytes int64
	HTTPClient       *http.Client
}

type ChatCompletionsClient struct {
	endpoint         string
	apiKey           string
	model            string
	provider         string
	maxResponseBytes int64
	httpClient       *http.Client
}

var _ assistant.LLMClient = (*ChatCompletionsClient)(nil)

func NewChatCompletionsClient(config ChatCompletionsConfig) (*ChatCompletionsClient, error) {
	endpoint, err := url.ParseRequestURI(strings.TrimSpace(config.Endpoint))
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return nil, errors.New("create chat completions client: endpoint must be an absolute http or https URL")
	}
	if strings.TrimSpace(config.APIKey) == "" {
		return nil, errors.New("create chat completions client: API key is required")
	}
	if strings.TrimSpace(config.Model) == "" {
		return nil, errors.New("create chat completions client: model is required")
	}
	if strings.TrimSpace(config.Provider) == "" {
		return nil, errors.New("create chat completions client: provider is required")
	}
	if config.MaxResponseBytes < 1 {
		return nil, errors.New("create chat completions client: max response bytes must be positive")
	}
	if config.HTTPClient == nil {
		return nil, errors.New("create chat completions client: HTTP client is required")
	}

	return &ChatCompletionsClient{
		endpoint:         endpoint.String(),
		apiKey:           config.APIKey,
		model:            config.Model,
		provider:         config.Provider,
		maxResponseBytes: config.MaxResponseBytes,
		httpClient:       config.HTTPClient,
	}, nil
}

type chatCompletionsRequest struct {
	Model          string                        `json:"model"`
	Messages       []chatCompletionsMessage      `json:"messages"`
	Temperature    float64                       `json:"temperature"`
	ResponseFormat chatCompletionsResponseFormat `json:"response_format"`
	Thinking       *chatCompletionsThinking      `json:"thinking,omitempty"`
}

type chatCompletionsMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionsResponseFormat struct {
	Type string `json:"type"`
}

type chatCompletionsThinking struct {
	Type string `json:"type"`
}

type chatCompletionsResponse struct {
	Model   string                  `json:"model"`
	Choices []chatCompletionsChoice `json:"choices"`
	Usage   chatCompletionsUsage    `json:"usage"`
}

type chatCompletionsChoice struct {
	Message chatCompletionsMessage `json:"message"`
}

type chatCompletionsUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func (c *ChatCompletionsClient) ParseIntent(
	ctx context.Context,
	message string,
) (assistant.IntentResult, assistant.LLMUsage, error) {
	requestBody, err := json.Marshal(chatCompletionsRequest{
		Model: c.model,
		Messages: []chatCompletionsMessage{
			{Role: "system", Content: assistant.SystemPrompt()},
			{Role: "user", Content: message},
		},
		Temperature:    0,
		ResponseFormat: chatCompletionsResponseFormat{Type: "json_object"},
		Thinking:       thinkingConfig(c.provider),
	})
	if err != nil {
		return assistant.IntentResult{}, assistant.LLMUsage{}, assistant.WrapError(assistant.CodeInternal, err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(requestBody))
	if err != nil {
		return assistant.IntentResult{}, assistant.LLMUsage{}, assistant.WrapError(assistant.CodeInternal, err)
	}
	request.Header.Set("Authorization", "Bearer "+c.apiKey)
	request.Header.Set("Content-Type", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
			return assistant.IntentResult{}, assistant.LLMUsage{}, assistant.WrapError(assistant.CodeRequestTimeout, err)
		}
		return assistant.IntentResult{}, assistant.LLMUsage{}, assistant.WrapError(
			assistant.CodeLLMUnavailable,
			errors.New("chat completions request failed"),
		)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return assistant.IntentResult{}, assistant.LLMUsage{}, assistant.WrapError(
			assistant.CodeLLMUnavailable,
			fmt.Errorf("chat completions returned HTTP status %d", response.StatusCode),
		)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, c.maxResponseBytes+1))
	if err != nil {
		return assistant.IntentResult{}, assistant.LLMUsage{}, assistant.WrapError(
			assistant.CodeLLMUnavailable,
			errors.New("read chat completions response"),
		)
	}
	if int64(len(body)) > c.maxResponseBytes {
		return assistant.IntentResult{}, assistant.LLMUsage{}, assistant.WrapError(
			assistant.CodeInvalidModelResponse,
			errors.New("chat completions response exceeds size limit"),
		)
	}

	var providerResponse chatCompletionsResponse
	if err := json.Unmarshal(body, &providerResponse); err != nil {
		return assistant.IntentResult{}, assistant.LLMUsage{}, assistant.WrapError(
			assistant.CodeInvalidModelResponse,
			errors.New("decode chat completions response"),
		)
	}
	if len(providerResponse.Choices) == 0 || strings.TrimSpace(providerResponse.Choices[0].Message.Content) == "" {
		return assistant.IntentResult{}, assistant.LLMUsage{}, assistant.WrapError(
			assistant.CodeInvalidModelResponse,
			errors.New("chat completions response has no content"),
		)
	}
	if providerResponse.Usage.PromptTokens < 0 ||
		providerResponse.Usage.CompletionTokens < 0 ||
		providerResponse.Usage.TotalTokens < 0 {
		return assistant.IntentResult{}, assistant.LLMUsage{}, assistant.WrapError(
			assistant.CodeInvalidModelResponse,
			errors.New("chat completions response has negative usage"),
		)
	}

	result, err := assistant.ParseIntentResult([]byte(providerResponse.Choices[0].Message.Content))
	if err != nil {
		return assistant.IntentResult{}, assistant.LLMUsage{}, err
	}
	model := providerResponse.Model
	if strings.TrimSpace(model) == "" {
		model = c.model
	}
	usage := assistant.LLMUsage{
		Provider:         c.provider,
		Model:            model,
		PromptTokens:     providerResponse.Usage.PromptTokens,
		CompletionTokens: providerResponse.Usage.CompletionTokens,
		TotalTokens:      providerResponse.Usage.TotalTokens,
	}
	return result, usage, nil
}

func thinkingConfig(provider string) *chatCompletionsThinking {
	if strings.EqualFold(strings.TrimSpace(provider), "deepseek") {
		return &chatCompletionsThinking{Type: "disabled"}
	}
	return nil
}
