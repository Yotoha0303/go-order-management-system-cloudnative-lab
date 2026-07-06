package assistant

import (
	"context"
	"errors"
	"sync"
	"testing"
)

type mockLLMClient struct {
	mu             sync.Mutex
	result         IntentResult
	usage          LLMUsage
	err            error
	waitForContext bool
	parseFunc      func(context.Context, string) (IntentResult, LLMUsage, error)
	calls          int
	messages       []string
}

var _ LLMClient = (*mockLLMClient)(nil)

func (m *mockLLMClient) ParseIntent(ctx context.Context, message string) (IntentResult, LLMUsage, error) {
	m.mu.Lock()
	m.calls++
	m.messages = append(m.messages, message)
	parseFunc := m.parseFunc
	waitForContext := m.waitForContext
	result, usage, err := m.result, m.usage, m.err
	m.mu.Unlock()

	if parseFunc != nil {
		return parseFunc(ctx, message)
	}
	if waitForContext {
		<-ctx.Done()
		return IntentResult{}, LLMUsage{}, ctx.Err()
	}
	return result, usage, err
}

func (m *mockLLMClient) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func (m *mockLLMClient) receivedMessages() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.messages...)
}

func TestMockLLMClientSupportsContextTimeout(t *testing.T) {
	client := &mockLLMClient{waitForContext: true}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := client.ParseIntent(ctx, "message")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ParseIntent error = %v, want context.Canceled", err)
	}
	if client.callCount() != 1 {
		t.Fatalf("call count = %d, want 1", client.callCount())
	}
	if got := client.receivedMessages(); len(got) != 1 || got[0] != "message" {
		t.Fatalf("messages = %v", got)
	}
}
