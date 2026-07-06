package assistant

import (
	"strings"
	"testing"
)

func TestSystemPromptIsVersionedAndContainsOnlyMVPTools(t *testing.T) {
	if PromptVersion == "" {
		t.Fatal("PromptVersion must not be empty")
	}
	prompt := SystemPrompt()
	for _, required := range []string{`"intent"`, `"arguments"`} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("prompt does not contain %q", required)
		}
	}
	for _, intent := range supportedIntents {
		if !strings.Contains(prompt, string(intent)) {
			t.Fatalf("prompt does not contain supported intent %q", intent)
		}
	}
	for _, forbidden := range []string{"delete_order", "update_inventory", "refund_order"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("prompt contains forbidden write intent %q", forbidden)
		}
	}
}

func TestSupportedIntentsMatchProductionToolsAndPrompt(t *testing.T) {
	registry, err := NewToolRegistry(
		&LowStockTool{},
		&OrderStatusSummaryTool{},
	)
	if err != nil {
		t.Fatalf("NewToolRegistry: %v", err)
	}
	if len(registry.tools) != len(supportedIntents) {
		t.Fatalf("registered tools = %d, supported intents = %d", len(registry.tools), len(supportedIntents))
	}
	for _, intent := range supportedIntents {
		if _, ok := registry.tools[intent]; !ok {
			t.Errorf("production tool is not registered for %q", intent)
		}
		if !strings.Contains(SystemPrompt(), string(intent)) {
			t.Errorf("system prompt is missing %q", intent)
		}
	}
}

func TestParseIntentResult(t *testing.T) {
	raw := []byte(`{"intent":"get_low_stock_products","arguments":{"threshold":10}}`)
	result, err := ParseIntentResult(raw)
	if err != nil {
		t.Fatalf("ParseIntentResult: %v", err)
	}
	if result.Intent != IntentGetLowStockProducts || string(result.Arguments) != `{"threshold":10}` {
		t.Fatalf("result = %+v", result)
	}
}

func TestParseIntentResultRejectsInvalidModelOutput(t *testing.T) {
	tests := []string{
		``,
		`null`,
		`[]`,
		"```json\n{\"intent\":\"get_low_stock_products\",\"arguments\":{}}\n```",
		`{"intent":"get_low_stock_products","arguments":{}} trailing`,
		`{"intent":"get_low_stock_products","arguments":{},"answer":"extra"}`,
		`{"intent":"get_low_stock_products","intent":"get_order_status_summary","arguments":{}}`,
		`{"intent":"get_low_stock_products"}`,
		`{"arguments":{}}`,
		`{"intent":"get_low_stock_products","arguments":null}`,
		`{"intent":"get_low_stock_products","arguments":[]}`,
		`{"intent":"get_low_stock_products","arguments":{"limit":1,"limit":2}}`,
	}

	for _, raw := range tests {
		t.Run(raw, func(t *testing.T) {
			_, err := ParseIntentResult([]byte(raw))
			if !IsCode(err, CodeInvalidModelResponse) {
				t.Fatalf("ParseIntentResult error = %v, want %s", err, CodeInvalidModelResponse)
			}
		})
	}
}

func TestParseIntentResultRejectsUnknownIntent(t *testing.T) {
	_, err := ParseIntentResult([]byte(`{"intent":"delete_order","arguments":{}}`))
	if !IsCode(err, CodeUnknownIntent) {
		t.Fatalf("ParseIntentResult error = %v, want %s", err, CodeUnknownIntent)
	}
}
