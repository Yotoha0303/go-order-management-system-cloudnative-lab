package middleware_test

import (
	"context"
	"encoding/json"
	"errors"
	"go-order-management-system/internal/middleware"
	"go-order-management-system/internal/response"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestTimeoutHandler_PassesThroughCompletedResponse(t *testing.T) {
	var hasDeadline bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hasDeadline = r.Context().Deadline()
		w.Header().Set("X-Test", "ok")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"result":"created"}`)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	middleware.TimeoutHandler(next, time.Second).ServeHTTP(recorder, request)

	if !hasDeadline {
		t.Fatal("expected request context to have a deadline")
	}
	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, recorder.Code)
	}
	if recorder.Header().Get("X-Test") != "ok" {
		t.Fatalf("expected response header to be preserved")
	}
	if recorder.Body.String() != `{"result":"created"}` {
		t.Fatalf("unexpected response body: %s", recorder.Body.String())
	}
}

func TestTimeoutHandler_ReturnsTimeoutResponseAndCancelsContext(t *testing.T) {
	contextErr := make(chan error, 1)
	writeErr := make(chan error, 1)
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblockHandler := func() {
		releaseOnce.Do(func() { close(release) })
	}
	defer unblockHandler()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		contextErr <- r.Context().Err()
		<-release
		_, err := io.WriteString(w, "late response")
		writeErr <- err
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/slow", nil)
	request.Header.Set(middleware.RequestIDHeader, "timeout-test-request")
	middleware.TimeoutHandler(next, 20*time.Millisecond).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("unexpected content type: %q", got)
	}
	if got := recorder.Header().Get(middleware.RequestIDHeader); got != "timeout-test-request" {
		t.Fatalf("unexpected request ID: %q", got)
	}

	var body response.Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode timeout response: %v", err)
	}
	if body.Code != response.CodeRequestTimeout || body.Msg != "request timeout" {
		t.Fatalf("unexpected timeout response: %+v", body)
	}

	select {
	case err := <-contextErr:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected deadline exceeded, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("handler context was not canceled")
	}

	unblockHandler()
	select {
	case err := <-writeErr:
		if !errors.Is(err, http.ErrHandlerTimeout) {
			t.Fatalf("expected ErrHandlerTimeout, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed-out handler did not exit")
	}
}

func TestTimeoutHandler_Disabled(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := r.Context().Deadline(); ok {
			t.Error("did not expect a deadline when timeout is disabled")
		}
		w.WriteHeader(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	middleware.TimeoutHandler(next, 0).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, recorder.Code)
	}
}
