package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"

	"go-order-management-system/internal/assistant"
	"go-order-management-system/internal/auth"
	"go-order-management-system/internal/middleware"
	"go-order-management-system/internal/response"

	"github.com/gin-gonic/gin"
)

const maxAssistantRequestBytes = 4096

type AssistantChatService interface {
	Chat(ctx context.Context, input assistant.ChatInput) (assistant.ChatResponse, error)
}

type AssistantHandler struct {
	service AssistantChatService
}

func NewAssistantHandler(service AssistantChatService) (*AssistantHandler, error) {
	if isNilAssistantService(service) {
		return nil, errors.New("create assistant handler: service must not be nil")
	}
	return &AssistantHandler{service: service}, nil
}

func (h *AssistantHandler) Chat(c *gin.Context) {
	request, err := decodeAssistantRequest(c)
	if err != nil {
		writeAssistantError(c, assistant.NewError(assistant.CodeInvalidRequest))
		return
	}
	message := strings.TrimSpace(request.Message)
	if message == "" || len([]byte(message)) > assistant.MaxChatMessageBytes {
		writeAssistantError(c, assistant.NewError(assistant.CodeInvalidRequest))
		return
	}

	identity, ok := auth.IdentityFromContext(c.Request.Context())
	if !ok {
		writeAssistantError(c, assistant.NewError(assistant.CodeUnauthorized))
		return
	}
	result, err := h.service.Chat(c.Request.Context(), assistant.ChatInput{
		RequestID: middleware.GetRequestID(c),
		UserID:    identity.UserID,
		Message:   message,
	})
	if err != nil {
		writeAssistantError(c, err)
		return
	}
	response.Success(c, result)
}

func decodeAssistantRequest(c *gin.Context) (assistant.ChatRequest, error) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxAssistantRequestBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()

	var request assistant.ChatRequest
	if err := decoder.Decode(&request); err != nil {
		return assistant.ChatRequest{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return assistant.ChatRequest{}, errors.New("request contains multiple JSON values")
		}
		return assistant.ChatRequest{}, err
	}
	return request, nil
}

func writeAssistantError(c *gin.Context, err error) {
	code := assistant.CodeOf(err)
	httpStatus, responseCode := assistantResponseCode(code)
	response.Fail(c, httpStatus, responseCode, assistant.PublicMessage(err))
}

func assistantResponseCode(code assistant.ErrorCode) (int, int) {
	switch code {
	case assistant.CodeInvalidRequest, assistant.CodeInvalidArguments:
		return http.StatusBadRequest, response.CodeAssistantInvalidRequest
	case assistant.CodeUnauthorized:
		return http.StatusUnauthorized, response.CodeTokenUserInvalid
	case assistant.CodeForbidden:
		return http.StatusForbidden, response.CodePermissionDenied
	case assistant.CodeUnknownIntent:
		return http.StatusUnprocessableEntity, response.CodeAssistantUnknownIntent
	case assistant.CodeInvalidModelResponse:
		return http.StatusBadGateway, response.CodeAssistantInvalidModelResponse
	case assistant.CodeLLMUnavailable:
		return http.StatusBadGateway, response.CodeAssistantLLMUnavailable
	case assistant.CodeRequestTimeout:
		return http.StatusGatewayTimeout, response.CodeRequestTimeout
	case assistant.CodeToolExecutionFailed:
		return http.StatusInternalServerError, response.CodeAssistantToolFailed
	default:
		return http.StatusInternalServerError, response.CodeAssistantInternalError
	}
}

func isNilAssistantService(service AssistantChatService) bool {
	if service == nil {
		return true
	}
	value := reflect.ValueOf(service)
	return value.Kind() == reflect.Pointer && value.IsNil()
}
