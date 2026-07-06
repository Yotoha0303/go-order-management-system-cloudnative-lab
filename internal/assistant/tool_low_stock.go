package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

const (
	defaultLowStockThreshold = 10
	defaultLowStockLimit     = 20
	maxLowStockThreshold     = 100000
	maxLowStockLimit         = 100
)

type LowStockTool struct {
	repository InventoryQueryRepository
}

type lowStockArguments struct {
	Threshold *int `json:"threshold"`
	Limit     *int `json:"limit"`
}

type LowStockResult struct {
	Threshold int               `json:"threshold"`
	Count     int               `json:"count"`
	Items     []LowStockProduct `json:"items"`
}

func NewLowStockTool(repository InventoryQueryRepository) (*LowStockTool, error) {
	if isNilInterface(repository) {
		return nil, errors.New("create low stock tool: repository must not be nil")
	}
	return &LowStockTool{repository: repository}, nil
}

func (t *LowStockTool) Name() Intent {
	return IntentGetLowStockProducts
}

func (t *LowStockTool) Execute(ctx context.Context, rawArguments json.RawMessage) (ToolResult, error) {
	if err := ctx.Err(); err != nil {
		return ToolResult{}, timeoutError(err)
	}

	arguments, err := parseLowStockArguments(rawArguments)
	if err != nil {
		return ToolResult{}, err
	}
	items, err := t.repository.ListLowStockProducts(ctx, arguments.threshold, arguments.limit)
	if err != nil {
		return ToolResult{}, repositoryError(err)
	}
	if len(items) > arguments.limit {
		return ToolResult{}, WrapError(
			CodeToolExecutionFailed,
			fmt.Errorf("inventory repository returned %d items for limit %d", len(items), arguments.limit),
		)
	}
	for _, item := range items {
		if item.Stock > int64(arguments.threshold) {
			return ToolResult{}, WrapError(
				CodeToolExecutionFailed,
				fmt.Errorf("inventory repository returned stock %d above threshold %d", item.Stock, arguments.threshold),
			)
		}
	}
	if items == nil {
		items = make([]LowStockProduct, 0)
	}

	data := LowStockResult{
		Threshold: arguments.threshold,
		Count:     len(items),
		Items:     items,
	}
	answer := fmt.Sprintf("发现 %d 个库存低于或等于 %d 的商品。", len(items), arguments.threshold)
	if len(items) == 0 {
		answer = fmt.Sprintf("未发现库存低于或等于 %d 的商品。", arguments.threshold)
	}
	return ToolResult{Answer: answer, Data: data}, nil
}

type normalizedLowStockArguments struct {
	threshold int
	limit     int
}

func parseLowStockArguments(raw json.RawMessage) (normalizedLowStockArguments, error) {
	var arguments lowStockArguments
	if err := decodeStrictObject(raw, &arguments); err != nil {
		return normalizedLowStockArguments{}, WrapError(CodeInvalidArguments, err)
	}

	threshold := defaultLowStockThreshold
	if arguments.Threshold != nil {
		threshold = *arguments.Threshold
	}
	if threshold < 0 || threshold > maxLowStockThreshold {
		return normalizedLowStockArguments{}, WrapError(
			CodeInvalidArguments,
			fmt.Errorf("threshold must be between 0 and %d", maxLowStockThreshold),
		)
	}

	limit := defaultLowStockLimit
	if arguments.Limit != nil {
		limit = *arguments.Limit
	}
	if limit < 1 || limit > maxLowStockLimit {
		return normalizedLowStockArguments{}, WrapError(
			CodeInvalidArguments,
			fmt.Errorf("limit must be between 1 and %d", maxLowStockLimit),
		)
	}
	return normalizedLowStockArguments{threshold: threshold, limit: limit}, nil
}

func repositoryError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return timeoutError(err)
	}
	return WrapError(CodeToolExecutionFailed, err)
}

func timeoutError(err error) error {
	return WrapError(CodeRequestTimeout, err)
}
