package model

import "time"

type UserRole struct {
	ID        int64     `gorm:"type:bigint;primaryKey;autoIncrement" json:"id"`
	RoleID    int64     `gorm:"type:bigint;not null;index:idx_user_roles_role_id" json:"role_id"`
	UserID    int64     `gorm:"type:bigint;not null;uniqueIndex:uk_user_id" json:"user_id"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (UserRole) TableName() string {
	return "user_roles"
}
