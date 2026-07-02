package dao

import (
	"context"
	"time"

	"go-order-management-system/internal/model"

	"gorm.io/gorm"
)

func CreateUser(ctx context.Context, db *gorm.DB, user *model.User) error {
	return db.WithContext(ctx).Create(user).Error
}

func GetUserByUsername(ctx context.Context, db *gorm.DB, username string) (*model.User, error) {
	var user model.User
	return &user, db.WithContext(ctx).Where("username = ?", username).First(&user).Error
}

func GetUserByID(ctx context.Context, db *gorm.DB, id int64) (*model.User, error) {
	var user model.User
	return &user, db.WithContext(ctx).Where("id = ?", id).First(&user).Error
}

func UpdateNicknameByID(ctx context.Context, db *gorm.DB, id int64, nickname string) error {
	return db.WithContext(ctx).Model(&model.User{}).Where("id = ?", id).Update("nickname", nickname).Error
}

func UpdateLastLoginAtByID(ctx context.Context, db *gorm.DB, id int64, at time.Time) error {
	return db.WithContext(ctx).Model(&model.User{}).Where("id = ?", id).Update("last_login_at", at).Error
}

func UpdateUserPassword(ctx context.Context, db *gorm.DB, id int64, oldHash, newHash string) (int64, error) {
	result := db.WithContext(ctx).Model(&model.User{}).
		Where("id = ? AND password_hash = ?", id, oldHash).
		Update("password_hash", newHash)
	return result.RowsAffected, result.Error
}
