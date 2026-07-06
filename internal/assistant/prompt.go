package assistant

import (
	"encoding/json"
	"errors"
	"strings"
)

const PromptVersion = "intent-v1"

const systemPrompt = `你是订单运营查询的意图分类器。你只能选择下列只读意图之一，并提取参数：

1. get_low_stock_products
   arguments: {"threshold": integer 0..100000, "limit": integer 1..100}
   threshold 默认 10，limit 默认 20。

2. get_order_status_summary
   arguments: {"days": integer 1..90}
   days 默认 7。

只输出一个 JSON object，顶层必须且只能包含 intent 和 arguments：
{"intent":"get_low_stock_products","arguments":{"threshold":10,"limit":20}}

禁止输出 Markdown、解释、代码块、SQL、写操作或列表。无法匹配时仍不得创造新意图。`

func SystemPrompt() string {
	return systemPrompt
}

func ParseIntentResult(raw []byte) (IntentResult, error) {
	var result IntentResult
	if err := decodeStrictObject(json.RawMessage(raw), &result); err != nil {
		return IntentResult{}, WrapError(CodeInvalidModelResponse, err)
	}
	if strings.TrimSpace(string(result.Intent)) == "" {
		return IntentResult{}, WrapError(CodeInvalidModelResponse, errors.New("intent is required"))
	}
	if !result.Intent.Valid() {
		return IntentResult{}, NewError(CodeUnknownIntent)
	}

	var arguments map[string]json.RawMessage
	if err := decodeStrictObject(result.Arguments, &arguments); err != nil {
		return IntentResult{}, WrapError(CodeInvalidModelResponse, errors.New("arguments must be a JSON object"))
	}
	return result, nil
}
