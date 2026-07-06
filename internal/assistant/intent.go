package assistant

import "encoding/json"

type Intent string

const (
	IntentGetLowStockProducts   Intent = "get_low_stock_products"
	IntentGetOrderStatusSummary Intent = "get_order_status_summary"
)

var supportedIntents = [...]Intent{
	IntentGetLowStockProducts,
	IntentGetOrderStatusSummary,
}

func (i Intent) Valid() bool {
	for _, supported := range supportedIntents {
		if i == supported {
			return true
		}
	}
	return false
}

type IntentResult struct {
	Intent    Intent          `json:"intent"`
	Arguments json.RawMessage `json:"arguments"`
}
