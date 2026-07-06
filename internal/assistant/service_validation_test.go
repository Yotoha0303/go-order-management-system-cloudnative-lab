package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestAssistantServiceRejectsInvalidInputBeforeLLM(t *testing.T) {
	llm := &mockLLMClient{}
	registry, _ := NewToolRegistry()
	logs := &fakeCallLogRepository{}
	service := newTestAssistantService(t, llm, registry, logs, 100*time.Millisecond)

	for _, input := range []ChatInput{
		{UserID: 1, Message: "  "},
		{UserID: 0, Message: "query"},
		{UserID: 1, Message: string(make([]byte, MaxChatMessageBytes+1))},
	} {
		_, err := service.Chat(context.Background(), input)
		if err == nil {
			t.Fatalf("Chat(%+v): want error", input)
		}
	}
	if llm.callCount() != 0 || logs.calls != 0 {
		t.Fatalf("LLM calls = %d, log calls = %d, want 0", llm.callCount(), logs.calls)
	}
}

func TestNewAssistantServiceRejectsInvalidDependencies(t *testing.T) {
	registry, _ := NewToolRegistry()
	validLLM := &mockLLMClient{}
	validLogs := &fakeCallLogRepository{}
	validConfig := ServiceConfig{
		LLM:               validLLM,
		Registry:          registry,
		CallLogs:          validLogs,
		Timeout:           time.Second,
		Now:               time.Now,
		NewRequestID:      GenerateRequestID,
		Logger:            slog.New(slog.NewTextHandler(io.Discard, nil)),
		LogPersistTimeout: time.Second,
	}
	var typedNilLLM *mockLLMClient
	var typedNilLogs *fakeCallLogRepository
	tests := []struct {
		name   string
		mutate func(*ServiceConfig)
	}{
		{name: "nil LLM", mutate: func(c *ServiceConfig) { c.LLM = nil }},
		{name: "typed nil LLM", mutate: func(c *ServiceConfig) { c.LLM = typedNilLLM }},
		{name: "nil registry", mutate: func(c *ServiceConfig) { c.Registry = nil }},
		{name: "nil logs", mutate: func(c *ServiceConfig) { c.CallLogs = nil }},
		{name: "typed nil logs", mutate: func(c *ServiceConfig) { c.CallLogs = typedNilLogs }},
		{name: "zero timeout", mutate: func(c *ServiceConfig) { c.Timeout = 0 }},
		{name: "large timeout", mutate: func(c *ServiceConfig) { c.Timeout = 2 * time.Minute }},
		{name: "nil clock", mutate: func(c *ServiceConfig) { c.Now = nil }},
		{name: "nil request ID", mutate: func(c *ServiceConfig) { c.NewRequestID = nil }},
		{name: "nil logger", mutate: func(c *ServiceConfig) { c.Logger = nil }},
		{name: "zero log timeout", mutate: func(c *ServiceConfig) { c.LogPersistTimeout = 0 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := validConfig
			tt.mutate(&config)
			if _, err := NewAssistantService(config); err == nil {
				t.Fatal("NewAssistantService: want error")
			}
		})
	}
}

func TestAssistantServiceRejectsNegativeUsageBeforeTool(t *testing.T) {
	llm := &mockLLMClient{
		result: IntentResult{Intent: IntentGetLowStockProducts, Arguments: json.RawMessage(`{}`)},
		usage:  LLMUsage{PromptTokens: -1, CompletionTokens: 2, TotalTokens: 1},
	}
	tool := &fakeTool{name: IntentGetLowStockProducts}
	registry, _ := NewToolRegistry(tool)
	logs := &fakeCallLogRepository{}
	service := newTestAssistantService(t, llm, registry, logs, 100*time.Millisecond)

	_, err := service.Chat(context.Background(), ChatInput{UserID: 1, Message: "query"})
	if !IsCode(err, CodeInvalidModelResponse) {
		t.Fatalf("Chat error = %v, want %s", err, CodeInvalidModelResponse)
	}
	if tool.called != 0 {
		t.Fatalf("tool calls = %d, want 0", tool.called)
	}
	log := logs.lastLog()
	if log.PromptTokens != 0 || log.CompletionTokens != 0 || log.TotalTokens != 0 {
		t.Fatalf("unsafe usage persisted: %+v", log)
	}
}

func TestAssistantServiceRequestIDFailureStopsBeforeLLM(t *testing.T) {
	llm := &mockLLMClient{}
	registry, _ := NewToolRegistry()
	logs := &fakeCallLogRepository{}
	service, err := NewAssistantService(ServiceConfig{
		LLM:               llm,
		Registry:          registry,
		CallLogs:          logs,
		Timeout:           time.Second,
		Now:               time.Now,
		NewRequestID:      func() (string, error) { return "", errors.New("random source failed") },
		Logger:            slog.New(slog.NewTextHandler(io.Discard, nil)),
		LogPersistTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewAssistantService: %v", err)
	}

	_, err = service.Chat(context.Background(), ChatInput{UserID: 1, Message: "query"})
	if !IsCode(err, CodeInternal) {
		t.Fatalf("Chat error = %v, want %s", err, CodeInternal)
	}
	if llm.callCount() != 0 || logs.calls != 0 {
		t.Fatalf("LLM calls = %d, log calls = %d", llm.callCount(), logs.calls)
	}
}

func TestAssistantServiceCanceledParentStopsBeforeLLM(t *testing.T) {
	llm := &mockLLMClient{}
	registry, _ := NewToolRegistry()
	logs := &fakeCallLogRepository{}
	service := newTestAssistantService(t, llm, registry, logs, time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := service.Chat(ctx, ChatInput{UserID: 1, Message: "query"})
	if !IsCode(err, CodeRequestTimeout) {
		t.Fatalf("Chat error = %v, want %s", err, CodeRequestTimeout)
	}
	if llm.callCount() != 0 || logs.calls != 0 {
		t.Fatalf("LLM calls = %d, log calls = %d", llm.callCount(), logs.calls)
	}
}

func TestGenerateRequestID(t *testing.T) {
	first, err := GenerateRequestID()
	if err != nil {
		t.Fatalf("GenerateRequestID: %v", err)
	}
	second, err := GenerateRequestID()
	if err != nil {
		t.Fatalf("GenerateRequestID: %v", err)
	}
	if len(first) != 36 || first[:4] != "req_" || first == second {
		t.Fatalf("request IDs = %q and %q", first, second)
	}
}
