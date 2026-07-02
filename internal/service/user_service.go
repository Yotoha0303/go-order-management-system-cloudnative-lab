package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"go-order-management-system/internal/apperror"
	"go-order-management-system/internal/dao"
	"go-order-management-system/internal/model"
	"go-order-management-system/internal/request"
	"go-order-management-system/internal/response"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	ErrUsernameAlreadyExists  = apperror.New(http.StatusConflict, response.CodeUsernameAlreadyExists, "用户名已存在")
	ErrUsernameInvalid        = apperror.New(http.StatusBadRequest, response.CodeParameterError, "用户名长度必须为 3 到 64 个字符")
	ErrPasswordInvalid        = apperror.New(http.StatusBadRequest, response.CodeParameterError, "密码长度必须为 6 到 72 个字符")
	ErrInvalidCredentials     = apperror.New(http.StatusUnauthorized, response.CodeLoginFailed, "用户名或密码错误")
	ErrUserDisabled           = apperror.New(http.StatusForbidden, response.CodeUserDisabled, "用户已被禁用")
	ErrUserNotFound           = apperror.New(http.StatusNotFound, response.CodeUserNotFound, "用户不存在")
	ErrInvalidUserID          = apperror.New(http.StatusBadRequest, response.CodeInvalidUserID, "无效的用户 ID")
	ErrNicknameInvalid        = apperror.New(http.StatusBadRequest, response.CodeNicknameInvalid, "昵称不能为空且不能超过 64 个字符")
	ErrPasswordUnchanged      = apperror.New(http.StatusBadRequest, response.CodeUserPasswordNoDifference, "新密码不能与原密码相同")
	ErrOldPasswordIncorrect   = apperror.New(http.StatusBadRequest, response.CodeUpdateUserPasswordFailed, "原密码错误")
	ErrPasswordUpdateConflict = apperror.New(http.StatusConflict, response.CodeUpdateUserPasswordFailed, "密码已被其他请求修改，请重试")
	ErrDatabaseNotInitialized = apperror.New(http.StatusInternalServerError, response.CodeDatabaseNotInitialized, "数据库未初始化")
)

type UserService struct{ db *gorm.DB }

func NewUserService(db *gorm.DB) *UserService { return &UserService{db: db} }

func (s *UserService) ensureDB() error {
	if s == nil || s.db == nil {
		return ErrDatabaseNotInitialized
	}
	return nil
}

func validatePassword(password string) error {
	if len(password) < 6 || len(password) > 72 {
		return ErrPasswordInvalid
	}
	return nil
}

func (s *UserService) Register(ctx context.Context, req request.RegisterRequest) error {
	if err := s.ensureDB(); err != nil {
		return err
	}
	username := strings.TrimSpace(req.Username)
	if len(username) < 3 || len(username) > 64 {
		return ErrUsernameInvalid
	}
	if err := validatePassword(req.Password); err != nil {
		return err
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return apperror.Wrap(http.StatusInternalServerError, response.CodeRegisterFailed, "注册失败", err)
	}
	user := &model.User{
		Username:     username,
		PasswordHash: string(passwordHash),
		Nickname:     username,
		Status:       model.UserStatusActive,
	}
	if err := dao.CreateUser(ctx, s.db, user); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return ErrUsernameAlreadyExists
		}
		return apperror.Wrap(http.StatusInternalServerError, response.CodeRegisterFailed, "注册失败", err)
	}
	return nil
}

func (s *UserService) Login(ctx context.Context, req request.LoginRequest) (*model.User, error) {
	if err := s.ensureDB(); err != nil {
		return nil, err
	}
	user, err := dao.GetUserByUsername(ctx, s.db, strings.TrimSpace(req.Username))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, apperror.Wrap(http.StatusInternalServerError, response.CodeLoginFailed, "登录失败", err)
	}
	if user.Status != model.UserStatusActive {
		return nil, ErrUserDisabled
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, ErrInvalidCredentials
	}
	at := time.Now()
	if err := dao.UpdateLastLoginAtByID(ctx, s.db, user.ID, at); err != nil {
		return nil, apperror.Wrap(http.StatusInternalServerError, response.CodeLoginFailed, "登录失败", err)
	}
	user.LastLoginAt = &at
	return user, nil
}

func (s *UserService) GetProfile(ctx context.Context, userID int64) (*model.User, error) {
	if err := s.ensureDB(); err != nil {
		return nil, err
	}
	if userID <= 0 {
		return nil, ErrInvalidUserID
	}
	user, err := dao.GetUserByID(ctx, s.db, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, apperror.Wrap(http.StatusInternalServerError, response.CodeUserNotFound, "查询用户失败", err)
	}
	if user.Status != model.UserStatusActive {
		return nil, ErrUserDisabled
	}
	return user, nil
}

func (s *UserService) UpdateNickname(ctx context.Context, userID int64, nickname string) error {
	if err := s.ensureDB(); err != nil {
		return err
	}
	nickname = strings.TrimSpace(nickname)
	if userID <= 0 {
		return ErrInvalidUserID
	}
	if nickname == "" || len(nickname) > 64 {
		return ErrNicknameInvalid
	}
	if _, err := s.GetProfile(ctx, userID); err != nil {
		return err
	}
	if err := dao.UpdateNicknameByID(ctx, s.db, userID, nickname); err != nil {
		return apperror.Wrap(http.StatusInternalServerError, response.CodeUpdateNicknameFailed, "修改昵称失败", err)
	}
	return nil
}

func (s *UserService) UpdatePassword(ctx context.Context, userID int64, req request.UpdatePasswordRequest) error {
	if err := s.ensureDB(); err != nil {
		return err
	}
	if userID <= 0 {
		return ErrInvalidUserID
	}
	if req.OldPassword == req.NewPassword {
		return ErrPasswordUnchanged
	}
	if err := validatePassword(req.NewPassword); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		user, err := dao.GetUserByID(ctx, tx, userID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrUserNotFound
			}
			return err
		}
		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.OldPassword)); err != nil {
			return ErrOldPasswordIncorrect
		}
		newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		rows, err := dao.UpdateUserPassword(ctx, tx, userID, user.PasswordHash, string(newHash))
		if err != nil {
			return apperror.Wrap(http.StatusInternalServerError, response.CodeUpdateUserPasswordFailed, "修改密码失败", err)
		}
		if rows != 1 {
			return ErrPasswordUpdateConflict
		}
		return nil
	})
}
