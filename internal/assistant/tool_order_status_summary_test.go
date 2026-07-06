package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"testing"
	"time"
)

type fakeOrderRepository struct {
	counts []OrderStatusCount
	err    error
	calls  int
	from   time.Time
	to     time.Time
}

func (r *fakeOrderRepository) SummarizeOrderStatus(
	_ context.Context,
	from, to time.Time,
) ([]OrderStatusCount, error) {
	r.calls++
	r.from = from
	r.to = to
	return r.counts, r.err
}

func TestOrderStatusSummaryToolUsesDefaultsAndFixedCalendarRange(t *testing.T) {
	location := time.FixedZone("CST", 8*60*60)
	now := time.Date(2026, 7, 4, 21, 30, 0, 0, location)
	repository := &fakeOrderRepository{counts: []OrderStatusCount{
		{Status: OrderStatusFinished, Count: 14},
		{Status: OrderStatusPending, Count: 8},
		{Status: OrderStatusPaid, Count: 20},
	}}
	tool, err := NewOrderStatusSummaryTool(repository, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewOrderStatusSummaryTool: %v", err)
	}

	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	wantFrom := time.Date(2026, 6, 28, 0, 0, 0, 0, location)
	wantTo := time.Date(2026, 7, 5, 0, 0, 0, 0, location)
	if !repository.from.Equal(wantFrom) || !repository.to.Equal(wantTo) {
		t.Fatalf("range = [%s, %s), want [%s, %s)", repository.from, repository.to, wantFrom, wantTo)
	}
	data := result.Data.(OrderStatusSummaryResult)
	if data.Days != 7 || data.Total != 42 || len(data.Counts) != 3 {
		t.Fatalf("result = %+v", data)
	}
	if data.Counts[0].Status != OrderStatusPending || data.Counts[2].Status != OrderStatusFinished {
		t.Fatalf("counts not in stable status order: %+v", data.Counts)
	}
}

func TestNewOrderStatusSummaryToolRejectsNilDependencies(t *testing.T) {
	var typedNil *fakeOrderRepository
	for _, repository := range []OrderQueryRepository{nil, typedNil} {
		if _, err := NewOrderStatusSummaryTool(repository, time.Now); err == nil {
			t.Fatal("NewOrderStatusSummaryTool nil repository: want error")
		}
	}
	if _, err := NewOrderStatusSummaryTool(&fakeOrderRepository{}, nil); err == nil {
		t.Fatal("NewOrderStatusSummaryTool nil clock: want error")
	}
}

func TestOrderStatusSummaryToolAcceptsDayBoundaries(t *testing.T) {
	for _, days := range []int{1, 90} {
		t.Run(strconv.Itoa(days), func(t *testing.T) {
			repository := &fakeOrderRepository{}
			tool, _ := NewOrderStatusSummaryTool(repository, time.Now)
			_, err := tool.Execute(context.Background(), json.RawMessage(`{"days":`+strconv.Itoa(days)+`}`))
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	}
}

func TestOrderStatusSummaryToolRejectsInvalidArgumentsBeforeQuery(t *testing.T) {
	for _, raw := range []string{
		`null`,
		`{"days":0}`,
		`{"days":91}`,
		`{"days":"7"}`,
		`{"from":"2026-01-01"}`,
		`{"days":7,"days":8}`,
	} {
		t.Run(raw, func(t *testing.T) {
			repository := &fakeOrderRepository{}
			tool, _ := NewOrderStatusSummaryTool(repository, time.Now)
			_, err := tool.Execute(context.Background(), json.RawMessage(raw))
			if !IsCode(err, CodeInvalidArguments) {
				t.Fatalf("Execute error = %v, want %s", err, CodeInvalidArguments)
			}
			if repository.calls != 0 {
				t.Fatalf("repository calls = %d, want 0", repository.calls)
			}
		})
	}
}

func TestOrderStatusSummaryToolRejectsInconsistentCounts(t *testing.T) {
	tests := []struct {
		name   string
		counts []OrderStatusCount
	}{
		{name: "unknown status", counts: []OrderStatusCount{{Status: "refunded", Count: 1}}},
		{name: "negative count", counts: []OrderStatusCount{{Status: OrderStatusPaid, Count: -1}}},
		{name: "duplicate status", counts: []OrderStatusCount{
			{Status: OrderStatusPaid, Count: 1},
			{Status: OrderStatusPaid, Count: 2},
		}},
		{name: "overflow", counts: []OrderStatusCount{
			{Status: OrderStatusPaid, Count: math.MaxInt64},
			{Status: OrderStatusPending, Count: 1},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool, _ := NewOrderStatusSummaryTool(&fakeOrderRepository{counts: tt.counts}, time.Now)
			_, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
			if !IsCode(err, CodeToolExecutionFailed) {
				t.Fatalf("Execute error = %v, want %s", err, CodeToolExecutionFailed)
			}
		})
	}
}

func TestOrderStatusSummaryToolReturnsNonNilEmptyCounts(t *testing.T) {
	tool, _ := NewOrderStatusSummaryTool(&fakeOrderRepository{}, time.Now)
	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	data := result.Data.(OrderStatusSummaryResult)
	if data.Counts == nil || data.Total != 0 {
		t.Fatalf("result = %+v, want non-nil empty counts", data)
	}
}

func TestOrderStatusSummaryToolClassifiesRepositoryError(t *testing.T) {
	tool, _ := NewOrderStatusSummaryTool(
		&fakeOrderRepository{err: errors.New("database detail")},
		time.Now,
	)
	_, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if !IsCode(err, CodeToolExecutionFailed) {
		t.Fatalf("Execute error = %v, want %s", err, CodeToolExecutionFailed)
	}
}

func TestOrderStatusSummaryToolClassifiesRepositoryDeadline(t *testing.T) {
	tool, _ := NewOrderStatusSummaryTool(
		&fakeOrderRepository{err: context.DeadlineExceeded},
		time.Now,
	)
	_, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if !IsCode(err, CodeRequestTimeout) {
		t.Fatalf("Execute error = %v, want %s", err, CodeRequestTimeout)
	}
}

func TestOrderStatusSummaryToolDoesNotQueryCanceledContext(t *testing.T) {
	repository := &fakeOrderRepository{}
	tool, _ := NewOrderStatusSummaryTool(repository, time.Now)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := tool.Execute(ctx, json.RawMessage(`{}`))
	if !IsCode(err, CodeRequestTimeout) || repository.calls != 0 {
		t.Fatalf("error = %v, calls = %d", err, repository.calls)
	}
}
