package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"go-order-management-system/config"
	"go-order-management-system/internal/app"
	"go-order-management-system/internal/response"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestNewHTTPServer_ConfiguresServerLimits(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{Port: 8082},
		HttpServer: config.HttpServer{Server: config.HttpServerConfig{
			ReadTimeOut:       time.Second,
			WriteTimeout:      2 * time.Second,
			IdleTimeout:       3 * time.Second,
			ReadHeaderTimeout: 4 * time.Second,
			MaxHeaderBytesKib: 128,
			Timeout:           5 * time.Second,
		}},
	}

	server := app.NewHTTPServer(&app.Deps{Config: cfg, Router: gin.New()})

	if server.Addr != ":8082" {
		t.Fatalf("unexpected server address: %s", server.Addr)
	}
	if server.ReadTimeout != time.Second || server.WriteTimeout != 2*time.Second {
		t.Fatalf("unexpected read/write timeouts: %s/%s", server.ReadTimeout, server.WriteTimeout)
	}
	if server.IdleTimeout != 3*time.Second || server.ReadHeaderTimeout != 4*time.Second {
		t.Fatalf("unexpected idle/header timeouts: %s/%s", server.IdleTimeout, server.ReadHeaderTimeout)
	}
	if server.MaxHeaderBytes != 128<<10 {
		t.Fatalf("unexpected max header bytes: %d", server.MaxHeaderBytes)
	}
}

func TestNewHTTPServer_EnforcesRequestTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	canceled := make(chan error, 1)
	finished := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblockHandler := func() {
		releaseOnce.Do(func() { close(release) })
	}
	defer unblockHandler()

	router := gin.New()
	router.GET("/slow", func(c *gin.Context) {
		<-c.Request.Context().Done()
		canceled <- c.Request.Context().Err()
		<-release
		close(finished)
	})

	cfg := &config.Config{
		HttpServer: config.HttpServer{Server: config.HttpServerConfig{Timeout: 20 * time.Millisecond}},
	}
	server := app.NewHTTPServer(&app.Deps{Config: cfg, Router: router})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/slow", nil)
	server.Handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, recorder.Code)
	}
	var body response.Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode timeout response: %v", err)
	}
	if body.Code != response.CodeRequestTimeout {
		t.Fatalf("expected timeout code %d, got %d", response.CodeRequestTimeout, body.Code)
	}

	select {
	case err := <-canceled:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected deadline exceeded, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("request context was not canceled")
	}

	unblockHandler()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("timed-out handler did not exit")
	}
}
