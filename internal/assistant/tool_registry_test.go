package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type fakeTool struct {
	name       Intent
	result     ToolResult
	err        error
	called     int
	contextKey any
	contextVal any
	arguments  json.RawMessage
}

func (t *fakeTool) Name() Intent {
	return t.name
}

func (t *fakeTool) Execute(ctx context.Context, arguments json.RawMessage) (ToolResult, error) {
	t.called++
	t.arguments = append(t.arguments[:0], arguments...)
	if t.contextKey != nil {
		t.contextVal = ctx.Value(t.contextKey)
	}
	return t.result, t.err
}

func TestToolRegistryExecute(t *testing.T) {
	type contextKey string
	key := contextKey("request")
	tool := &fakeTool{
		name:       IntentGetLowStockProducts,
		result:     ToolResult{Answer: "ok"},
		contextKey: key,
	}
	registry, err := NewToolRegistry(tool)
	if err != nil {
		t.Fatalf("NewToolRegistry: %v", err)
	}

	arguments := json.RawMessage(`{"threshold":10}`)
	result, err := registry.Execute(context.WithValue(context.Background(), key, "req-1"), tool.name, arguments)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Answer != "ok" || tool.called != 1 {
		t.Fatalf("unexpected execution: result=%+v called=%d", result, tool.called)
	}
	if string(tool.arguments) != string(arguments) {
		t.Fatalf("arguments = %s, want %s", tool.arguments, arguments)
	}
	if tool.contextVal != "req-1" {
		t.Fatalf("context value = %v, want req-1", tool.contextVal)
	}
}

func TestToolRegistryRejectsInvalidRegistrations(t *testing.T) {
	var typedNil *fakeTool
	tests := []struct {
		name  string
		tools []Tool
	}{
		{name: "nil", tools: []Tool{nil}},
		{name: "typed nil", tools: []Tool{typedNil}},
		{name: "unsupported name", tools: []Tool{&fakeTool{name: "delete_order"}}},
		{
			name: "duplicate",
			tools: []Tool{
				&fakeTool{name: IntentGetLowStockProducts},
				&fakeTool{name: IntentGetLowStockProducts},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewToolRegistry(tt.tools...); err == nil {
				t.Fatal("NewToolRegistry: want error")
			}
		})
	}
}

func TestToolRegistryRejectsUnknownIntent(t *testing.T) {
	registry, err := NewToolRegistry(&fakeTool{name: IntentGetLowStockProducts})
	if err != nil {
		t.Fatalf("NewToolRegistry: %v", err)
	}

	_, err = registry.Execute(context.Background(), IntentGetOrderStatusSummary, json.RawMessage(`{}`))
	if !IsCode(err, CodeUnknownIntent) {
		t.Fatalf("Execute error = %v, want %s", err, CodeUnknownIntent)
	}
}

func TestToolRegistryDoesNotExecuteCanceledContext(t *testing.T) {
	tool := &fakeTool{name: IntentGetLowStockProducts}
	registry, err := NewToolRegistry(tool)
	if err != nil {
		t.Fatalf("NewToolRegistry: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = registry.Execute(ctx, tool.name, json.RawMessage(`{}`))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute error = %v, want context.Canceled", err)
	}
	if tool.called != 0 {
		t.Fatalf("tool called %d times, want 0", tool.called)
	}
}
