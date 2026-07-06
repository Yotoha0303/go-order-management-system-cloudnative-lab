package model

import "time"

type AICallLog struct {
	ID               int64     `gorm:"primaryKey;autoIncrement;type:bigint"`
	RequestID        string    `gorm:"type:varchar(64);not null;uniqueIndex:uk_ai_call_logs_request_id"`
	UserID           int64     `gorm:"type:bigint;not null;index:idx_ai_call_logs_user_created,priority:1"`
	Intent           string    `gorm:"type:varchar(64);not null;default:''"`
	ToolName         string    `gorm:"type:varchar(64);not null;default:''"`
	Provider         string    `gorm:"type:varchar(64);not null;default:''"`
	Model            string    `gorm:"type:varchar(128);not null;default:''"`
	PromptTokens     int       `gorm:"type:int unsigned;not null;default:0"`
	CompletionTokens int       `gorm:"type:int unsigned;not null;default:0"`
	TotalTokens      int       `gorm:"type:int unsigned;not null;default:0"`
	LatencyMS        int64     `gorm:"type:bigint unsigned;not null;default:0"`
	Status           string    `gorm:"type:varchar(16);not null;index:idx_ai_call_logs_status_created,priority:1"`
	ErrorCode        string    `gorm:"type:varchar(64);not null;default:''"`
	CreatedAt        time.Time `gorm:"type:datetime(3);not null;default:CURRENT_TIMESTAMP(3);index:idx_ai_call_logs_user_created,priority:2;index:idx_ai_call_logs_status_created,priority:2"`
}

func (AICallLog) TableName() string {
	return "ai_call_logs"
}
