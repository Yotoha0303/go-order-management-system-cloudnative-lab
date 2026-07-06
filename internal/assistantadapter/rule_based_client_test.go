package assistantadapter

import (
	"context"
	"testing"

	"go-order-management-system/internal/assistant"
)

func TestRuleBasedClientRecognizesReadOnlyIntents(t *testing.T) {
	client := NewRuleBasedClient()
	for message, want := range map[string]assistant.Intent{
		"查询低库存":         assistant.IntentGetLowStockProducts,
		"order summary": assistant.IntentGetOrderStatusSummary,
	} {
		result, usage, err := client.ParseIntent(context.Background(), message)
		if err != nil {
			t.Fatalf("ParseIntent(%q): %v", message, err)
		}
		if result.Intent != want || usage.Provider != "mock" {
			t.Fatalf("result=%+v usage=%+v", result, usage)
		}
	}
}

func TestRuleBasedClientRejectsUnknownIntent(t *testing.T) {
	_, _, err := NewRuleBasedClient().ParseIntent(context.Background(), "天气")
	if !assistant.IsCode(err, assistant.CodeUnknownIntent) {
		t.Fatalf("error = %v", err)
	}
}
