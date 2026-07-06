package assistant

import "time"

type CallStatus string

const (
	CallStatusSuccess CallStatus = "success"
	CallStatusFailed  CallStatus = "failed"
)

type AICallLog struct {
	RequestID        string
	UserID           int64
	Intent           Intent
	ToolName         string
	Provider         string
	Model            string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	LatencyMS        int64
	Status           CallStatus
	ErrorCode        ErrorCode
	CreatedAt        time.Time
}
