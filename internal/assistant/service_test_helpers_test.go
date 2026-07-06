package assistant

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

type fakeCallLogRepository struct {
	mu     sync.Mutex
	logs   []AICallLog
	err    error
	calls  int
	ctxErr error
}

func (r *fakeCallLogRepository) Save(ctx context.Context, log AICallLog) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.ctxErr = ctx.Err()
	r.logs = append(r.logs, log)
	return r.err
}

func (r *fakeCallLogRepository) lastLog() AICallLog {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.logs[len(r.logs)-1]
}

func newTestAssistantService(
	t *testing.T,
	llm LLMClient,
	registry *ToolRegistry,
	logs CallLogRepository,
	timeout time.Duration,
) AssistantService {
	t.Helper()
	currentTime := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	service, err := NewAssistantService(ServiceConfig{
		LLM:               llm,
		Registry:          registry,
		CallLogs:          logs,
		Timeout:           timeout,
		Now:               func() time.Time { currentTime = currentTime.Add(time.Millisecond); return currentTime },
		NewRequestID:      func() (string, error) { return "req-generated", nil },
		Logger:            slog.New(slog.NewTextHandler(io.Discard, nil)),
		LogPersistTimeout: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewAssistantService: %v", err)
	}
	return service
}
