package assistant

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestDomainErrorClassificationSurvivesWrapping(t *testing.T) {
	cause := errors.New("database host and secret details")
	err := fmt.Errorf("service operation: %w", WrapError(CodeToolExecutionFailed, cause))

	if !IsCode(err, CodeToolExecutionFailed) {
		t.Fatalf("CodeOf(error) = %q, want %q", CodeOf(err), CodeToolExecutionFailed)
	}
	if !errors.Is(err, cause) {
		t.Fatal("wrapped cause is not available to internal error handling")
	}
	if got := PublicMessage(err); got != "业务数据查询失败" {
		t.Fatalf("PublicMessage(error) = %q", got)
	}
	if strings.Contains(PublicMessage(err), "database") || strings.Contains(PublicMessage(err), "secret") {
		t.Fatalf("public message exposed cause: %q", PublicMessage(err))
	}
}

func TestUnknownErrorUsesInternalCode(t *testing.T) {
	err := errors.New("unexpected details")
	if got := CodeOf(err); got != CodeInternal {
		t.Fatalf("CodeOf(error) = %q, want %q", got, CodeInternal)
	}
	if got := PublicMessage(err); got != "服务器内部错误" {
		t.Fatalf("PublicMessage(error) = %q", got)
	}
}

func TestUnknownCodeNormalizesToInternal(t *testing.T) {
	err := NewError(ErrorCode("future_code"))
	if got := CodeOf(err); got != CodeInternal {
		t.Fatalf("CodeOf(error) = %q, want %q", got, CodeInternal)
	}
}

func TestIntentValid(t *testing.T) {
	if !IntentGetLowStockProducts.Valid() || !IntentGetOrderStatusSummary.Valid() {
		t.Fatal("MVP intents must be valid")
	}
	if Intent("delete_order").Valid() {
		t.Fatal("write intent must not be valid")
	}
}
