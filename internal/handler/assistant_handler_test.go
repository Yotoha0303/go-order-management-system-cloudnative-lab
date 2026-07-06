package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go-order-management-system/internal/assistant"
	"go-order-management-system/internal/auth"
	"go-order-management-system/internal/middleware"
	"go-order-management-system/internal/response"

	"github.com/gin-gonic/gin"
)

type stubAssistantService struct {
	result assistant.ChatResponse
	err    error
	calls  int
	input  assistant.ChatInput
}

func (s *stubAssistantService) Chat(_ context.Context, input assistant.ChatInput) (assistant.ChatResponse, error) {
	s.calls++
	s.input = input
	return s.result, s.err
}

func TestAssistantHandlerChatSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &stubAssistantService{result: assistant.ChatResponse{
		RequestID: "req-1",
		Intent:    assistant.IntentGetLowStockProducts,
		Answer:    "ok",
		Data:      map[string]int{"count": 1},
	}}
	handler, err := NewAssistantHandler(service)
	if err != nil {
		t.Fatalf("NewAssistantHandler: %v", err)
	}
	engine := newAssistantHandlerEngine(handler, true)

	recorder := performAssistantRequest(engine, `{"message":"  查询低库存  "}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.calls != 1 || service.input.UserID != 7 || service.input.RequestID != "req-1" || service.input.Message != "查询低库存" {
		t.Fatalf("calls=%d input=%+v", service.calls, service.input)
	}
	var body response.Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != response.CodeSuccess {
		t.Fatalf("response=%+v", body)
	}
}

func TestAssistantHandlerRejectsInvalidRequestBeforeService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, body := range []string{
		``, `null`, `[]`, `{"message":""}`, `{"message":"query","extra":1}`,
		`{"message":"query"}{"message":"again"}`,
		`{"message":"` + strings.Repeat("x", assistant.MaxChatMessageBytes+1) + `"}`,
	} {
		service := &stubAssistantService{}
		handler, _ := NewAssistantHandler(service)
		recorder := performAssistantRequest(newAssistantHandlerEngine(handler, true), body)
		if recorder.Code != http.StatusBadRequest || service.calls != 0 {
			t.Fatalf("body=%q status=%d calls=%d", body, recorder.Code, service.calls)
		}
	}
}

func TestAssistantHandlerRequiresIdentity(t *testing.T) {
	service := &stubAssistantService{}
	handler, _ := NewAssistantHandler(service)
	recorder := performAssistantRequest(newAssistantHandlerEngine(handler, false), `{"message":"query"}`)
	if recorder.Code != http.StatusUnauthorized || service.calls != 0 {
		t.Fatalf("status=%d calls=%d", recorder.Code, service.calls)
	}
}

func TestAssistantHandlerMapsDomainErrors(t *testing.T) {
	tests := []struct {
		code       assistant.ErrorCode
		wantStatus int
		wantCode   int
	}{
		{assistant.CodeInvalidArguments, http.StatusBadRequest, response.CodeAssistantInvalidRequest},
		{assistant.CodeUnknownIntent, http.StatusUnprocessableEntity, response.CodeAssistantUnknownIntent},
		{assistant.CodeInvalidModelResponse, http.StatusBadGateway, response.CodeAssistantInvalidModelResponse},
		{assistant.CodeLLMUnavailable, http.StatusBadGateway, response.CodeAssistantLLMUnavailable},
		{assistant.CodeToolExecutionFailed, http.StatusInternalServerError, response.CodeAssistantToolFailed},
		{assistant.CodeRequestTimeout, http.StatusGatewayTimeout, response.CodeRequestTimeout},
		{assistant.CodeInternal, http.StatusInternalServerError, response.CodeAssistantInternalError},
	}
	for _, test := range tests {
		t.Run(string(test.code), func(t *testing.T) {
			service := &stubAssistantService{err: assistant.WrapError(test.code, errors.New("database secret"))}
			handler, _ := NewAssistantHandler(service)
			recorder := performAssistantRequest(newAssistantHandlerEngine(handler, true), `{"message":"query"}`)
			if recorder.Code != test.wantStatus || strings.Contains(recorder.Body.String(), "secret") {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			var body response.Response
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if body.Code != test.wantCode {
				t.Fatalf("code=%d want=%d", body.Code, test.wantCode)
			}
		})
	}
}

func newAssistantHandlerEngine(handler *AssistantHandler, withIdentity bool) *gin.Engine {
	engine := gin.New()
	engine.POST("/assistant", func(c *gin.Context) {
		c.Set(middleware.RequestKeyID, "req-1")
		if withIdentity {
			identity := auth.Identity{UserID: 7, Username: "admin"}
			c.Request = c.Request.WithContext(auth.ContextWithIdentity(c.Request.Context(), identity))
		}
		handler.Chat(c)
	})
	return engine
}

func performAssistantRequest(engine http.Handler, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/assistant", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	return recorder
}
