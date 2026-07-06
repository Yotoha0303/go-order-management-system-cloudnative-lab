package model

import "time"

const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

type Role struct {
	ID          int64     `gorm:"type:bigint;primaryKey;autoIncrement" json:"id"`
	RoleName    string    `gorm:"type:varchar(25);not null;uniqueIndex:uk_role_name" json:"role_name"`
	Description string    `gorm:"type:varchar(255);not null;default:''" json:"description"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (Role) TableName() string {
	return "roles"
}
