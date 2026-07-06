package dao

import (
	"context"

	"go-order-management-system/internal/model"

	"gorm.io/gorm"
)

func CreateUserRole(ctx context.Context, db *gorm.DB, userID, roleID int64) error {
	userRole := model.UserRole{
		UserID: userID,
		RoleID: roleID,
	}
	return db.WithContext(ctx).Create(&userRole).Error
}

func UserHasRole(ctx context.Context, db *gorm.DB, userID int64, roleName string) (bool, error) {
	var allowed bool
	err := db.WithContext(ctx).Raw(`
		SELECT EXISTS (
			SELECT 1
			FROM user_roles AS ur
			INNER JOIN roles AS r ON r.id = ur.role_id
			WHERE ur.user_id = ? AND r.role_name = ?
		)`, userID, roleName).Scan(&allowed).Error
	return allowed, err
}

func ListRoleNamesByUserID(ctx context.Context, db *gorm.DB, userID int64) ([]string, error) {
	roles := make([]string, 0, 1)
	err := db.WithContext(ctx).
		Table("user_roles AS ur").
		Select("r.role_name").
		Joins("INNER JOIN roles AS r ON r.id = ur.role_id").
		Where("ur.user_id = ?", userID).
		Order("r.role_name ASC").
		Pluck("r.role_name", &roles).Error
	return roles, err
}
