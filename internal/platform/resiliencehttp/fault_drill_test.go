package resiliencehttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestFaultDrillHTTPTimeoutCircuitRecovery exercises the production Executor
// against a real HTTP server. It deliberately causes response-header timeouts,
// verifies the circuit rejects without network I/O, then restores the upstream
// and proves half-open and closed-state recovery.
func TestFaultDrillHTTPTimeoutCircuitRecovery(t *testing.T) {
	var slow atomic.Bool
	var calls atomic.Int64
	slow.Store(true)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		if slow.Load() {
			time.Sleep(150 * time.Millisecond)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	client := NewHTTPClient(TransportConfig{
		ConnectTimeout:        100 * time.Millisecond,
		ResponseHeaderTimeout: 40 * time.Millisecond,
		TotalTimeout:          80 * time.Millisecond,
	})
	executor := NewExecutor(client, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	executor.breakerConfig = CircuitBreakerConfig{
		FailureThreshold:  2,
		OpenInterval:      120 * time.Millisecond,
		HalfOpenMaxProbes: 1,
	}
	policy := RetryPolicy{MaxAttempts: 1}
	factory := func(ctx context.Context) (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	}

	started := time.Now()
	for attempt := 0; attempt < 2; attempt++ {
		response, err := executor.Do(context.Background(), "fault-upstream", "probe", policy, factory)
		if response != nil {
			_ = response.Body.Close()
		}
		if err == nil {
			t.Fatalf("timeout attempt %d unexpectedly succeeded", attempt+1)
		}
	}
	timeoutElapsed := time.Since(started)
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected exactly two timed-out network calls, got %d", got)
	}

	openStarted := time.Now()
	response, err := executor.Do(context.Background(), "fault-upstream", "probe", policy, factory)
	if response != nil {
		_ = response.Body.Close()
	}
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected circuit-open rejection, got %v", err)
	}
	openElapsed := time.Since(openStarted)
	if got := calls.Load(); got != 2 {
		t.Fatalf("open circuit performed network I/O; calls=%d", got)
	}
	if openElapsed >= 40*time.Millisecond {
		t.Fatalf("open-circuit rejection was not immediate: %s", openElapsed)
	}

	slow.Store(false)
	time.Sleep(150 * time.Millisecond)

	halfOpen, err := executor.Do(context.Background(), "fault-upstream", "probe", policy, factory)
	if err != nil {
		t.Fatalf("half-open recovery probe failed: %v", err)
	}
	_ = halfOpen.Body.Close()

	closed, err := executor.Do(context.Background(), "fault-upstream", "probe", policy, factory)
	if err != nil {
		t.Fatalf("closed circuit request failed after recovery: %v", err)
	}
	_ = closed.Body.Close()
	if got := calls.Load(); got != 4 {
		t.Fatalf("expected two failures plus two recovery calls, got %d", got)
	}

	if output := os.Getenv("FAULT_DRILL_HTTP_OUTPUT"); output != "" {
		document := map[string]any{
			"schema_version":              1,
			"timeout_calls":               2,
			"timeout_elapsed_ms":          timeoutElapsed.Milliseconds(),
			"circuit_open_rejection_ms":   openElapsed.Milliseconds(),
			"network_calls_while_open":    0,
			"half_open_recovery":          "passed",
			"closed_state_after_recovery": "passed",
			"total_upstream_calls":        calls.Load(),
		}
		if writeErr := writeFaultDrillEvidence(output, document); writeErr != nil {
			t.Fatalf("write fault-drill evidence: %v", writeErr)
		}
	}
}

func writeFaultDrillEvidence(output string, document map[string]any) error {
	outputPath := filepath.Clean(output)
	if !filepath.IsAbs(outputPath) {
		if workspace := strings.TrimSpace(os.Getenv("GITHUB_WORKSPACE")); workspace != "" {
			outputPath = filepath.Join(workspace, outputPath)
		}
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o750); err != nil {
		return fmt.Errorf("create evidence directory: %w", err)
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal evidence: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(outputPath, data, 0o600); err != nil {
		return fmt.Errorf("write evidence file: %w", err)
	}
	return nil
}

func TestWriteFaultDrillEvidenceAnchorsRelativePathToWorkspace(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("GITHUB_WORKSPACE", workspace)
	relative := filepath.Join("fault-evidence", "http", "evidence.json")
	if err := writeFaultDrillEvidence(relative, map[string]any{"schema_version": 1}); err != nil {
		t.Fatalf("write workspace-anchored evidence: %v", err)
	}
	output := filepath.Join(workspace, relative)
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read workspace-anchored evidence: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("decode workspace-anchored evidence: %v", err)
	}
	if document["schema_version"] != float64(1) {
		t.Fatalf("unexpected evidence document: %v", document)
	}
}
