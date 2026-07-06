package dao

import (
	"context"
	"go-order-management-system/internal/model"

	"gorm.io/gorm"
)

func GetRoleByID(ctx context.Context, db *gorm.DB, id int64) (*model.Role, error) {
	var role model.Role
	return &role, db.WithContext(ctx).Where("id = ?", id).First(&role).Error
}

func GetRoleByRoleName(ctx context.Context, db *gorm.DB, roleName string) (*model.Role, error) {
	var role model.Role
	return &role, db.WithContext(ctx).Where("role_name = ?", roleName).First(&role).Error
}
