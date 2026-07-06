package service

import (
	"context"

	"go-order-management-system/internal/dao"

	"gorm.io/gorm"
)

type AuthorizationService struct {
	db *gorm.DB
}

func NewAuthorizationService(db *gorm.DB) *AuthorizationService {
	return &AuthorizationService{db: db}
}

func (s *AuthorizationService) HasRole(ctx context.Context, userID int64, roleName string) (bool, error) {
	if s == nil || s.db == nil {
		return false, ErrDatabaseNotInitialized
	}
	return dao.UserHasRole(ctx, s.db, userID, roleName)
}
