package service_test

import (
	"context"
	"errors"
	"testing"

	"go-order-management-system/internal/dao"
	"go-order-management-system/internal/model"
	"go-order-management-system/internal/request"
	"go-order-management-system/internal/service"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestUserServiceRegisterAssignsDefaultRole(t *testing.T) {
	db := setupTestDB(t)
	userService := service.NewUserService(db)
	username := "registered-" + uuid.NewString()

	if err := userService.Register(context.Background(), request.RegisterRequest{
		Username: username,
		Password: "password123",
	}); err != nil {
		t.Fatalf("register user failed: %v", err)
	}

	user, err := dao.GetUserByUsername(context.Background(), db, username)
	if err != nil {
		t.Fatalf("query registered user failed: %v", err)
	}
	allowed, err := dao.UserHasRole(context.Background(), db, user.ID, model.RoleUser)
	if err != nil {
		t.Fatalf("query registered user role failed: %v", err)
	}
	if !allowed {
		t.Fatal("expected registered user to have the user role")
	}

	loggedInUser, err := userService.Login(context.Background(), request.LoginRequest{
		Username: username,
		Password: "password123",
	})
	if err != nil {
		t.Fatalf("login registered user failed: %v", err)
	}
	if len(loggedInUser.Roles) != 1 || loggedInUser.Roles[0] != model.RoleUser {
		t.Fatalf("unexpected login roles: %v", loggedInUser.Roles)
	}
}

func TestUserServiceRegisterRollsBackWithoutDefaultRole(t *testing.T) {
	db := setupTestDB(t)
	if err := db.Where("role_name = ?", model.RoleUser).Delete(&model.Role{}).Error; err != nil {
		t.Fatalf("delete default role failed: %v", err)
	}
	userService := service.NewUserService(db)
	username := "rollback-" + uuid.NewString()

	err := userService.Register(context.Background(), request.RegisterRequest{
		Username: username,
		Password: "password123",
	})
	if !errors.Is(err, service.ErrDefaultRoleNotFound) {
		t.Fatalf("expected missing default role error, got %v", err)
	}

	_, err = dao.GetUserByUsername(context.Background(), db, username)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected user creation to roll back, got %v", err)
	}
}

func TestUserServiceUpdatePasswordInvalidatesOldPassword(t *testing.T) {
	db := setupTestDB(t)
	userService := service.NewUserService(db)
	username := "password-" + uuid.NewString()
	const oldPassword = "old-password-123"
	const newPassword = "new-password-456"

	if err := userService.Register(context.Background(), request.RegisterRequest{
		Username: username,
		Password: oldPassword,
	}); err != nil {
		t.Fatalf("register user failed: %v", err)
	}
	user, err := dao.GetUserByUsername(context.Background(), db, username)
	if err != nil {
		t.Fatalf("query registered user failed: %v", err)
	}

	if err := userService.UpdatePassword(context.Background(), user.ID, request.UpdatePasswordRequest{
		OldPassword: oldPassword,
		NewPassword: newPassword,
	}); err != nil {
		t.Fatalf("update password failed: %v", err)
	}

	_, err = userService.Login(context.Background(), request.LoginRequest{
		Username: username,
		Password: oldPassword,
	})
	if !errors.Is(err, service.ErrInvalidCredentials) {
		t.Fatalf("expected old password to be rejected, got %v", err)
	}
	if _, err := userService.Login(context.Background(), request.LoginRequest{
		Username: username,
		Password: newPassword,
	}); err != nil {
		t.Fatalf("expected new password login to succeed, got %v", err)
	}
}
