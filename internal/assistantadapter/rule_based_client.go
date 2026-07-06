package assistantadapter

import (
	"context"
	"encoding/json"
	"strings"

	"go-order-management-system/internal/assistant"
)

type RuleBasedClient struct{}

var _ assistant.LLMClient = (*RuleBasedClient)(nil)

func NewRuleBasedClient() *RuleBasedClient {
	return &RuleBasedClient{}
}

func (c *RuleBasedClient) ParseIntent(
	ctx context.Context,
	message string,
) (assistant.IntentResult, assistant.LLMUsage, error) {
	if err := ctx.Err(); err != nil {
		return assistant.IntentResult{}, assistant.LLMUsage{}, err
	}
	normalized := strings.ToLower(message)
	usage := assistant.LLMUsage{Provider: "mock", Model: "rule-based-v1"}
	switch {
	case strings.Contains(normalized, "库存"), strings.Contains(normalized, "stock"):
		return assistant.IntentResult{
			Intent:    assistant.IntentGetLowStockProducts,
			Arguments: json.RawMessage(`{}`),
		}, usage, nil
	case strings.Contains(normalized, "订单"), strings.Contains(normalized, "order"):
		return assistant.IntentResult{
			Intent:    assistant.IntentGetOrderStatusSummary,
			Arguments: json.RawMessage(`{}`),
		}, usage, nil
	default:
		return assistant.IntentResult{}, usage, assistant.NewError(assistant.CodeUnknownIntent)
	}
}
