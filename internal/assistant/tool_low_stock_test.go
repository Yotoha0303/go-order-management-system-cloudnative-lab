package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type fakeInventoryRepository struct {
	items     []LowStockProduct
	err       error
	calls     int
	threshold int
	limit     int
}

func (r *fakeInventoryRepository) ListLowStockProducts(
	_ context.Context,
	threshold, limit int,
) ([]LowStockProduct, error) {
	r.calls++
	r.threshold = threshold
	r.limit = limit
	return r.items, r.err
}

func TestLowStockToolUsesDefaultsAndReturnsStableResult(t *testing.T) {
	repository := &fakeInventoryRepository{items: []LowStockProduct{
		{ProductID: 1, Name: "Product 1", Stock: 3},
	}}
	tool, err := NewLowStockTool(repository)
	if err != nil {
		t.Fatalf("NewLowStockTool: %v", err)
	}

	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	data, ok := result.Data.(LowStockResult)
	if !ok {
		t.Fatalf("result data type = %T", result.Data)
	}
	if repository.threshold != 10 || repository.limit != 20 {
		t.Fatalf("repository arguments = (%d, %d), want (10, 20)", repository.threshold, repository.limit)
	}
	if data.Threshold != 10 || data.Count != 1 || len(data.Items) != 1 {
		t.Fatalf("result data = %+v", data)
	}
}

func TestNewLowStockToolRejectsNilRepositories(t *testing.T) {
	var typedNil *fakeInventoryRepository
	for _, repository := range []InventoryQueryRepository{nil, typedNil} {
		if _, err := NewLowStockTool(repository); err == nil {
			t.Fatal("NewLowStockTool: want error")
		}
	}
}

func TestLowStockToolAcceptsExplicitBoundaryValues(t *testing.T) {
	repository := &fakeInventoryRepository{}
	tool, _ := NewLowStockTool(repository)

	_, err := tool.Execute(context.Background(), json.RawMessage(`{"threshold":0,"limit":100}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if repository.threshold != 0 || repository.limit != 100 {
		t.Fatalf("repository arguments = (%d, %d)", repository.threshold, repository.limit)
	}
}

func TestLowStockToolRejectsInvalidArgumentsBeforeQuery(t *testing.T) {
	tests := []string{
		``,
		`null`,
		`[]`,
		`{"threshold":-1}`,
		`{"threshold":100001}`,
		`{"limit":0}`,
		`{"limit":101}`,
		`{"limit":"20"}`,
		`{"unknown":1}`,
		`{"limit":10,"limit":20}`,
		`{} {}`,
	}

	for _, raw := range tests {
		t.Run(raw, func(t *testing.T) {
			repository := &fakeInventoryRepository{}
			tool, _ := NewLowStockTool(repository)
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

func TestLowStockToolReturnsNonNilEmptyItems(t *testing.T) {
	tool, _ := NewLowStockTool(&fakeInventoryRepository{})
	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	data := result.Data.(LowStockResult)
	if data.Items == nil || len(data.Items) != 0 || data.Count != 0 {
		t.Fatalf("result data = %+v, want non-nil empty items", data)
	}
}

func TestLowStockToolClassifiesRepositoryFailures(t *testing.T) {
	tests := []struct {
		name     string
		repoErr  error
		wantCode ErrorCode
	}{
		{name: "query error", repoErr: errors.New("database secret"), wantCode: CodeToolExecutionFailed},
		{name: "deadline", repoErr: context.DeadlineExceeded, wantCode: CodeRequestTimeout},
		{name: "canceled", repoErr: context.Canceled, wantCode: CodeRequestTimeout},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool, _ := NewLowStockTool(&fakeInventoryRepository{err: tt.repoErr})
			_, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
			if !IsCode(err, tt.wantCode) {
				t.Fatalf("Execute error = %v, want %s", err, tt.wantCode)
			}
			if PublicMessage(err) == tt.repoErr.Error() {
				t.Fatal("public message exposed repository error")
			}
		})
	}
}

func TestLowStockToolRejectsInconsistentRepositoryResult(t *testing.T) {
	repository := &fakeInventoryRepository{items: []LowStockProduct{
		{ProductID: 1, Stock: 11},
	}}
	tool, _ := NewLowStockTool(repository)
	_, err := tool.Execute(context.Background(), json.RawMessage(`{"threshold":10}`))
	if !IsCode(err, CodeToolExecutionFailed) {
		t.Fatalf("Execute error = %v, want %s", err, CodeToolExecutionFailed)
	}
}

func TestLowStockToolDoesNotQueryCanceledContext(t *testing.T) {
	repository := &fakeInventoryRepository{}
	tool, _ := NewLowStockTool(repository)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := tool.Execute(ctx, json.RawMessage(`{}`))
	if !IsCode(err, CodeRequestTimeout) {
		t.Fatalf("Execute error = %v, want %s", err, CodeRequestTimeout)
	}
	if repository.calls != 0 {
		t.Fatalf("repository calls = %d, want 0", repository.calls)
	}
}
