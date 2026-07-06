package assistant

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestAssistantServiceChatSuccess(t *testing.T) {
	llm := &mockLLMClient{
		result: IntentResult{
			Intent:    IntentGetLowStockProducts,
			Arguments: json.RawMessage(`{"threshold":5}`),
		},
		usage: LLMUsage{
			Provider:         "provider",
			Model:            "model",
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	}
	tool := &fakeTool{
		name:   IntentGetLowStockProducts,
		result: ToolResult{Answer: "发现 1 个商品", Data: map[string]int{"count": 1}},
	}
	registry, _ := NewToolRegistry(tool)
	logs := &fakeCallLogRepository{}
	service := newTestAssistantService(t, llm, registry, logs, 100*time.Millisecond)

	response, err := service.Chat(context.Background(), ChatInput{
		RequestID: "req-fixed",
		UserID:    1,
		Message:   "  查询低库存  ",
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if response.RequestID != "req-fixed" || response.Intent != IntentGetLowStockProducts || response.Answer != "发现 1 个商品" {
		t.Fatalf("response = %+v", response)
	}
	if llm.callCount() != 1 || tool.called != 1 {
		t.Fatalf("LLM calls = %d, tool calls = %d", llm.callCount(), tool.called)
	}
	if got := llm.receivedMessages()[0]; got != "查询低库存" {
		t.Fatalf("LLM message = %q", got)
	}
	log := logs.lastLog()
	if log.RequestID != response.RequestID ||
		log.UserID != 1 ||
		log.Intent != IntentGetLowStockProducts ||
		log.ToolName != string(IntentGetLowStockProducts) ||
		log.Provider != "provider" ||
		log.Model != "model" ||
		log.PromptTokens != 10 ||
		log.CompletionTokens != 5 ||
		log.TotalTokens != 15 ||
		log.LatencyMS != 1 ||
		log.Status != CallStatusSuccess ||
		log.ErrorCode != "" ||
		logs.calls != 1 {
		t.Fatalf("call log = %+v", log)
	}
}
