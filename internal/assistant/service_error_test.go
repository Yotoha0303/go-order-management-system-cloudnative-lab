package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestAssistantServiceDoesNotExecuteUnknownIntent(t *testing.T) {
	llm := &mockLLMClient{result: IntentResult{Intent: "delete_order", Arguments: json.RawMessage(`{}`)}}
	tool := &fakeTool{name: IntentGetLowStockProducts}
	registry, _ := NewToolRegistry(tool)
	logs := &fakeCallLogRepository{}
	service := newTestAssistantService(t, llm, registry, logs, 100*time.Millisecond)

	_, err := service.Chat(context.Background(), ChatInput{UserID: 1, Message: "delete"})
	if !IsCode(err, CodeUnknownIntent) {
		t.Fatalf("Chat error = %v, want %s", err, CodeUnknownIntent)
	}
	if tool.called != 0 {
		t.Fatalf("tool called %d times, want 0", tool.called)
	}
	if log := logs.lastLog(); log.Status != CallStatusFailed || log.ErrorCode != CodeUnknownIntent {
		t.Fatalf("call log = %+v", log)
	}
}

func TestAssistantServiceClassifiesRawLLMError(t *testing.T) {
	llm := &mockLLMClient{
		err: errors.New("provider secret"),
		usage: LLMUsage{
			Provider:         "provider",
			Model:            "model",
			PromptTokens:     10,
			CompletionTokens: 2,
			TotalTokens:      12,
		},
	}
	registry, _ := NewToolRegistry()
	logs := &fakeCallLogRepository{}
	service := newTestAssistantService(t, llm, registry, logs, 100*time.Millisecond)

	_, err := service.Chat(context.Background(), ChatInput{UserID: 1, Message: "query"})
	if !IsCode(err, CodeLLMUnavailable) {
		t.Fatalf("Chat error = %v, want %s", err, CodeLLMUnavailable)
	}
	log := logs.lastLog()
	if log.ErrorCode != CodeLLMUnavailable || log.Status != CallStatusFailed ||
		log.Provider != "provider" || log.Model != "model" || log.TotalTokens != 12 || logs.calls != 1 {
		t.Fatalf("call log = %+v, calls = %d", log, logs.calls)
	}
}

func TestAssistantServiceTimesOutLLM(t *testing.T) {
	llm := &mockLLMClient{waitForContext: true}
	registry, _ := NewToolRegistry()
	logs := &fakeCallLogRepository{}
	service := newTestAssistantService(t, llm, registry, logs, 10*time.Millisecond)

	_, err := service.Chat(context.Background(), ChatInput{UserID: 1, Message: "query"})
	if !IsCode(err, CodeRequestTimeout) {
		t.Fatalf("Chat error = %v, want %s", err, CodeRequestTimeout)
	}
	if logs.lastLog().ErrorCode != CodeRequestTimeout {
		t.Fatalf("call log = %+v", logs.lastLog())
	}
	if logs.ctxErr != nil {
		t.Fatalf("call log received canceled context: %v", logs.ctxErr)
	}
}

func TestAssistantServicePreservesClassifiedLLMErrors(t *testing.T) {
	for _, code := range []ErrorCode{CodeInvalidModelResponse, CodeUnknownIntent, CodeLLMUnavailable} {
		t.Run(string(code), func(t *testing.T) {
			llm := &mockLLMClient{err: NewError(code)}
			tool := &fakeTool{name: IntentGetLowStockProducts}
			registry, _ := NewToolRegistry(tool)
			logs := &fakeCallLogRepository{}
			service := newTestAssistantService(t, llm, registry, logs, 100*time.Millisecond)

			_, err := service.Chat(context.Background(), ChatInput{UserID: 1, Message: "query"})
			if !IsCode(err, code) {
				t.Fatalf("Chat error = %v, want %s", err, code)
			}
			if tool.called != 0 || logs.lastLog().ErrorCode != code {
				t.Fatalf("tool calls = %d, log = %+v", tool.called, logs.lastLog())
			}
		})
	}
}

func TestAssistantServiceClassifiesToolFailures(t *testing.T) {
	tests := []struct {
		name     string
		toolErr  error
		wantCode ErrorCode
	}{
		{name: "raw error", toolErr: errors.New("repository details"), wantCode: CodeToolExecutionFailed},
		{name: "invalid arguments", toolErr: NewError(CodeInvalidArguments), wantCode: CodeInvalidArguments},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			llm := &mockLLMClient{result: IntentResult{
				Intent:    IntentGetLowStockProducts,
				Arguments: json.RawMessage(`{}`),
			}}
			tool := &fakeTool{name: IntentGetLowStockProducts, err: tt.toolErr}
			registry, _ := NewToolRegistry(tool)
			logs := &fakeCallLogRepository{}
			service := newTestAssistantService(t, llm, registry, logs, 100*time.Millisecond)

			_, err := service.Chat(context.Background(), ChatInput{UserID: 1, Message: "query"})
			if !IsCode(err, tt.wantCode) || logs.lastLog().ErrorCode != tt.wantCode {
				t.Fatalf("Chat error = %v, log = %+v, want %s", err, logs.lastLog(), tt.wantCode)
			}
		})
	}
}
