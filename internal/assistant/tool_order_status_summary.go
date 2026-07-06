package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"
)

const (
	defaultOrderSummaryDays = 7
	maxOrderSummaryDays     = 90
)

type OrderStatusSummaryTool struct {
	repository OrderQueryRepository
	now        func() time.Time
}

type orderStatusSummaryArguments struct {
	Days *int `json:"days"`
}

type OrderStatusSummaryResult struct {
	Days   int                `json:"days"`
	From   time.Time          `json:"from"`
	To     time.Time          `json:"to"`
	Total  int64              `json:"total"`
	Counts []OrderStatusCount `json:"counts"`
}

func NewOrderStatusSummaryTool(
	repository OrderQueryRepository,
	now func() time.Time,
) (*OrderStatusSummaryTool, error) {
	if isNilInterface(repository) {
		return nil, errors.New("create order status summary tool: repository must not be nil")
	}
	if isNilInterface(now) {
		return nil, errors.New("create order status summary tool: clock must not be nil")
	}
	return &OrderStatusSummaryTool{repository: repository, now: now}, nil
}

func (t *OrderStatusSummaryTool) Name() Intent {
	return IntentGetOrderStatusSummary
}

func (t *OrderStatusSummaryTool) Execute(
	ctx context.Context,
	rawArguments json.RawMessage,
) (ToolResult, error) {
	if err := ctx.Err(); err != nil {
		return ToolResult{}, timeoutError(err)
	}
	days, err := parseOrderStatusSummaryArguments(rawArguments)
	if err != nil {
		return ToolResult{}, err
	}

	now := t.now()
	to := startOfNextDay(now)
	from := to.AddDate(0, 0, -days)
	counts, err := t.repository.SummarizeOrderStatus(ctx, from, to)
	if err != nil {
		return ToolResult{}, repositoryError(err)
	}

	total, err := validateAndSortOrderStatusCounts(counts)
	if err != nil {
		return ToolResult{}, err
	}
	if counts == nil {
		counts = make([]OrderStatusCount, 0)
	}
	data := OrderStatusSummaryResult{
		Days:   days,
		From:   from,
		To:     to,
		Total:  total,
		Counts: counts,
	}
	answer := fmt.Sprintf("最近 %d 天共有 %d 个订单。", days, total)
	return ToolResult{Answer: answer, Data: data}, nil
}

func parseOrderStatusSummaryArguments(raw json.RawMessage) (int, error) {
	var arguments orderStatusSummaryArguments
	if err := decodeStrictObject(raw, &arguments); err != nil {
		return 0, WrapError(CodeInvalidArguments, err)
	}
	days := defaultOrderSummaryDays
	if arguments.Days != nil {
		days = *arguments.Days
	}
	if days < 1 || days > maxOrderSummaryDays {
		return 0, WrapError(
			CodeInvalidArguments,
			fmt.Errorf("days must be between 1 and %d", maxOrderSummaryDays),
		)
	}
	return days, nil
}

func startOfNextDay(now time.Time) time.Time {
	year, month, day := now.Date()
	return time.Date(year, month, day+1, 0, 0, 0, 0, now.Location())
}

func validateAndSortOrderStatusCounts(counts []OrderStatusCount) (int64, error) {
	seen := make(map[OrderStatus]struct{}, len(counts))
	var total int64
	for _, count := range counts {
		if !count.Status.Valid() {
			return 0, WrapError(
				CodeToolExecutionFailed,
				fmt.Errorf("order repository returned unknown status %q", count.Status),
			)
		}
		if _, exists := seen[count.Status]; exists {
			return 0, WrapError(
				CodeToolExecutionFailed,
				fmt.Errorf("order repository returned duplicate status %q", count.Status),
			)
		}
		seen[count.Status] = struct{}{}
		if count.Count < 0 || count.Count > math.MaxInt64-total {
			return 0, WrapError(
				CodeToolExecutionFailed,
				fmt.Errorf("order repository returned invalid count %d for status %q", count.Count, count.Status),
			)
		}
		total += count.Count
	}
	sort.Slice(counts, func(i, j int) bool {
		return orderStatusRank(counts[i].Status) < orderStatusRank(counts[j].Status)
	})
	return total, nil
}

func orderStatusRank(status OrderStatus) int {
	switch status {
	case OrderStatusPending:
		return 0
	case OrderStatusPaid:
		return 1
	case OrderStatusFinished:
		return 2
	case OrderStatusCancelled:
		return 3
	default:
		return math.MaxInt
	}
}
