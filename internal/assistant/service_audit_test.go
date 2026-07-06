package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestAssistantServiceIgnoresCallLogFailure(t *testing.T) {
	llm := &mockLLMClient{result: IntentResult{
		Intent:    IntentGetLowStockProducts,
		Arguments: json.RawMessage(`{}`),
	}}
	tool := &fakeTool{name: IntentGetLowStockProducts, result: ToolResult{Answer: "ok"}}
	registry, _ := NewToolRegistry(tool)
	logs := &fakeCallLogRepository{err: errors.New("log database failed")}
	service := newTestAssistantService(t, llm, registry, logs, 100*time.Millisecond)

	response, err := service.Chat(context.Background(), ChatInput{UserID: 1, Message: "query"})
	if err != nil || response.Answer != "ok" {
		t.Fatalf("Chat response=%+v error=%v", response, err)
	}
}

func TestAssistantServiceCallLogFailureDoesNotReplaceBusinessError(t *testing.T) {
	llm := &mockLLMClient{err: NewError(CodeInvalidModelResponse)}
	registry, _ := NewToolRegistry()
	logs := &fakeCallLogRepository{err: errors.New("log database failed")}
	service := newTestAssistantService(t, llm, registry, logs, 100*time.Millisecond)

	_, err := service.Chat(context.Background(), ChatInput{UserID: 1, Message: "query"})
	if !IsCode(err, CodeInvalidModelResponse) {
		t.Fatalf("Chat error = %v, want %s", err, CodeInvalidModelResponse)
	}
	if logs.calls != 1 {
		t.Fatalf("log calls = %d, want 1", logs.calls)
	}
}

func TestAICallLogContainsNoSensitivePayloadFields(t *testing.T) {
	logType := reflect.TypeOf(AICallLog{})
	forbidden := map[string]struct{}{
		"message": {}, "content": {}, "prompt": {}, "toolarguments": {},
		"toolresult": {}, "apikey": {}, "authorization": {},
	}
	for i := 0; i < logType.NumField(); i++ {
		fieldName := strings.ToLower(logType.Field(i).Name)
		if _, exists := forbidden[fieldName]; exists {
			t.Fatalf("AICallLog contains forbidden payload field %q", logType.Field(i).Name)
		}
	}
}
