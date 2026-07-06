package assistant

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

const MaxChatMessageBytes = 2000

type AssistantService interface {
	Chat(ctx context.Context, input ChatInput) (ChatResponse, error)
}

type RequestIDGenerator func() (string, error)

type ServiceConfig struct {
	LLM               LLMClient
	Registry          *ToolRegistry
	CallLogs          CallLogRepository
	Timeout           time.Duration
	Now               func() time.Time
	NewRequestID      RequestIDGenerator
	Logger            *slog.Logger
	LogPersistTimeout time.Duration
}

type service struct {
	llm               LLMClient
	registry          *ToolRegistry
	callLogs          CallLogRepository
	timeout           time.Duration
	now               func() time.Time
	newRequestID      RequestIDGenerator
	logger            *slog.Logger
	logPersistTimeout time.Duration
}

var _ AssistantService = (*service)(nil)

func NewAssistantService(config ServiceConfig) (AssistantService, error) {
	if isNilInterface(config.LLM) {
		return nil, errors.New("create assistant service: LLM client must not be nil")
	}
	if config.Registry == nil {
		return nil, errors.New("create assistant service: tool registry must not be nil")
	}
	if isNilInterface(config.CallLogs) {
		return nil, errors.New("create assistant service: call log repository must not be nil")
	}
	if config.Timeout <= 0 || config.Timeout > time.Minute {
		return nil, errors.New("create assistant service: timeout must be greater than 0 and at most 1m")
	}
	if config.Now == nil {
		return nil, errors.New("create assistant service: clock must not be nil")
	}
	if config.NewRequestID == nil {
		return nil, errors.New("create assistant service: request ID generator must not be nil")
	}
	if config.Logger == nil {
		return nil, errors.New("create assistant service: logger must not be nil")
	}
	if config.LogPersistTimeout <= 0 || config.LogPersistTimeout > 5*time.Second {
		return nil, errors.New("create assistant service: log persist timeout must be greater than 0 and at most 5s")
	}
	return &service{
		llm:               config.LLM,
		registry:          config.Registry,
		callLogs:          config.CallLogs,
		timeout:           config.Timeout,
		now:               config.Now,
		newRequestID:      config.NewRequestID,
		logger:            config.Logger,
		logPersistTimeout: config.LogPersistTimeout,
	}, nil
}

func (s *service) Chat(ctx context.Context, input ChatInput) (response ChatResponse, returnErr error) {
	message := strings.TrimSpace(input.Message)
	if message == "" || len([]byte(message)) > MaxChatMessageBytes {
		return ChatResponse{}, NewError(CodeInvalidRequest)
	}
	if input.UserID <= 0 {
		return ChatResponse{}, NewError(CodeUnauthorized)
	}
	if err := ctx.Err(); err != nil {
		return ChatResponse{}, timeoutError(err)
	}

	requestID := strings.TrimSpace(input.RequestID)
	if requestID == "" {
		var err error
		requestID, err = s.newRequestID()
		if err != nil || strings.TrimSpace(requestID) == "" {
			return ChatResponse{}, WrapError(CodeInternal, errors.New("generate request ID"))
		}
	}

	startedAt := s.now()
	callLog := AICallLog{
		RequestID: requestID,
		UserID:    input.UserID,
		Status:    CallStatusFailed,
		CreatedAt: startedAt,
	}
	defer func() {
		finishedAt := s.now()
		latency := finishedAt.Sub(startedAt)
		if latency < 0 {
			latency = 0
		}
		callLog.LatencyMS = latency.Milliseconds()
		if returnErr == nil {
			callLog.Status = CallStatusSuccess
			callLog.ErrorCode = ""
		} else {
			callLog.Status = CallStatusFailed
			callLog.ErrorCode = CodeOf(returnErr)
		}
		s.persistCallLog(ctx, callLog)
	}()

	operationCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	intentResult, usage, err := s.llm.ParseIntent(operationCtx, message)
	callLog.Provider = usage.Provider
	callLog.Model = usage.Model
	if usageTokensValid(usage) {
		callLog.PromptTokens = usage.PromptTokens
		callLog.CompletionTokens = usage.CompletionTokens
		callLog.TotalTokens = usage.TotalTokens
	}
	if err != nil {
		return ChatResponse{}, classifyLLMError(operationCtx, err)
	}
	if !usageTokensValid(usage) {
		return ChatResponse{}, WrapError(CodeInvalidModelResponse, errors.New("LLM returned negative token usage"))
	}
	callLog.Intent = intentResult.Intent
	callLog.ToolName = string(intentResult.Intent)
	if !intentResult.Intent.Valid() {
		return ChatResponse{}, NewError(CodeUnknownIntent)
	}

	toolResult, err := s.registry.Execute(operationCtx, intentResult.Intent, intentResult.Arguments)
	if err != nil {
		return ChatResponse{}, classifyToolError(operationCtx, err)
	}
	return ChatResponse{
		RequestID: requestID,
		Intent:    intentResult.Intent,
		Answer:    toolResult.Answer,
		Data:      toolResult.Data,
	}, nil
}

func usageTokensValid(usage LLMUsage) bool {
	return usage.PromptTokens >= 0 && usage.CompletionTokens >= 0 && usage.TotalTokens >= 0
}

func (s *service) persistCallLog(parent context.Context, callLog AICallLog) {
	logCtx, cancel := context.WithTimeout(context.WithoutCancel(parent), s.logPersistTimeout)
	defer cancel()
	if err := s.callLogs.Save(logCtx, callLog); err != nil {
		s.logger.Error("save AI call log", "request_id", callLog.RequestID, "error", err)
	}
}

func classifyLLMError(ctx context.Context, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
		return timeoutError(err)
	}
	if CodeOf(err) != CodeInternal {
		return err
	}
	return WrapError(CodeLLMUnavailable, err)
}

func classifyToolError(ctx context.Context, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
		return timeoutError(err)
	}
	if CodeOf(err) != CodeInternal {
		return err
	}
	return WrapError(CodeToolExecutionFailed, err)
}

func GenerateRequestID() (string, error) {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	return "req_" + hex.EncodeToString(random), nil
}
