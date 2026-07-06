package assistant

import (
	"errors"
	"fmt"
)

type ErrorCode string

const (
	CodeInvalidRequest       ErrorCode = "invalid_request"
	CodeInvalidArguments     ErrorCode = "invalid_arguments"
	CodeUnauthorized         ErrorCode = "unauthorized"
	CodeForbidden            ErrorCode = "forbidden"
	CodeUnknownIntent        ErrorCode = "unknown_intent"
	CodeInvalidModelResponse ErrorCode = "invalid_model_response"
	CodeLLMUnavailable       ErrorCode = "llm_unavailable"
	CodeToolExecutionFailed  ErrorCode = "tool_execution_failed"
	CodeRequestTimeout       ErrorCode = "request_timeout"
	CodeInternal             ErrorCode = "internal_error"
)

var publicMessages = map[ErrorCode]string{
	CodeInvalidRequest:       "请求格式不合法",
	CodeInvalidArguments:     "请求参数不合法",
	CodeUnauthorized:         "请先登录",
	CodeForbidden:            "无权访问该功能",
	CodeUnknownIntent:        "暂不支持该查询",
	CodeInvalidModelResponse: "AI 服务返回了无效结果",
	CodeLLMUnavailable:       "AI 服务暂时不可用",
	CodeToolExecutionFailed:  "业务数据查询失败",
	CodeRequestTimeout:       "请求处理超时",
	CodeInternal:             "服务器内部错误",
}

type DomainError struct {
	code  ErrorCode
	cause error
}

func NewError(code ErrorCode) error {
	return &DomainError{code: normalizeCode(code)}
}

func WrapError(code ErrorCode, cause error) error {
	if cause == nil {
		return NewError(code)
	}
	return &DomainError{code: normalizeCode(code), cause: cause}
}

func (e *DomainError) Error() string {
	if e.cause == nil {
		return string(e.code)
	}
	return fmt.Sprintf("%s: %v", e.code, e.cause)
}

func (e *DomainError) Unwrap() error {
	return e.cause
}

func (e *DomainError) Code() ErrorCode {
	return e.code
}

func CodeOf(err error) ErrorCode {
	if err == nil {
		return ""
	}
	var domainErr *DomainError
	if errors.As(err, &domainErr) {
		return normalizeCode(domainErr.code)
	}
	return CodeInternal
}

func PublicMessage(err error) string {
	return publicMessages[CodeOf(err)]
}

func IsCode(err error, code ErrorCode) bool {
	return CodeOf(err) == code
}

func normalizeCode(code ErrorCode) ErrorCode {
	if _, ok := publicMessages[code]; !ok {
		return CodeInternal
	}
	return code
}
