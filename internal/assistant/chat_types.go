package assistant

type ChatRequest struct {
	Message string `json:"message"`
}

type ChatInput struct {
	RequestID string
	UserID    int64
	Message   string
}

type ChatResponse struct {
	RequestID string `json:"request_id"`
	Intent    Intent `json:"intent"`
	Answer    string `json:"answer"`
	Data      any    `json:"data"`
}

type ErrorResponse struct {
	RequestID string            `json:"request_id,omitempty"`
	Error     ErrorResponseBody `json:"error"`
}

type ErrorResponseBody struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}
