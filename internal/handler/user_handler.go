package handler

import (
	"context"
	"net/http"

	"go-order-management-system/internal/apperror"
	"go-order-management-system/internal/auth"
	"go-order-management-system/internal/middleware"
	"go-order-management-system/internal/model"
	"go-order-management-system/internal/request"
	"go-order-management-system/internal/response"

	"github.com/gin-gonic/gin"
)

type UserService interface {
	Register(context.Context, request.RegisterRequest) error
	Login(context.Context, request.LoginRequest) (*model.User, error)
	GetProfile(context.Context, int64) (*model.User, error)
	UpdateNickname(context.Context, int64, string) error
	UpdatePassword(context.Context, int64, request.UpdatePasswordRequest) error
}

type UserHandler struct {
	service       UserService
	generateToken func(int64, string) (string, error)
}

func NewUserHandler(service UserService, tokenManager *auth.TokenManager) *UserHandler {
	return &UserHandler{service: service, generateToken: tokenManager.GenerateAccessToken}
}

func (h *UserHandler) Register(c *gin.Context) {
	var req request.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, response.CodeParameterError, "请求参数错误")
		return
	}
	if err := h.service.Register(c.Request.Context(), req); err != nil {
		handleError(c, err, response.CodeRegisterFailed, "注册失败")
		return
	}
	response.Success(c, nil)
}

func (h *UserHandler) Login(c *gin.Context) {
	var req request.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, response.CodeParameterError, "请求参数错误")
		return
	}
	user, err := h.service.Login(c.Request.Context(), req)
	if err != nil {
		handleError(c, err, response.CodeLoginFailed, "登录失败")
		return
	}
	token, err := h.generateToken(user.ID, user.Username)
	if err != nil {
		handleError(c, apperror.Wrap(http.StatusInternalServerError, response.CodeTokenGenerateFailed, "生成访问令牌失败", err), response.CodeTokenGenerateFailed, "生成访问令牌失败")
		return
	}
	response.Success(c, response.TokenAndUserInfoResponse{AccessToken: token, User: userResponse(user)})
}

func (h *UserHandler) Me(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	user, err := h.service.GetProfile(c.Request.Context(), userID)
	if err != nil {
		handleError(c, err, response.CodeUserNotFound, "查询用户失败")
		return
	}
	response.Success(c, userResponse(user))
}

func (h *UserHandler) UpdateProfile(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req request.UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, response.CodeParameterError, "请求参数错误")
		return
	}
	if err := h.service.UpdateNickname(c.Request.Context(), userID, req.Nickname); err != nil {
		handleError(c, err, response.CodeUpdateNicknameFailed, "修改昵称失败")
		return
	}
	response.Success(c, nil)
}

func (h *UserHandler) UpdatePassword(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req request.UpdatePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, response.CodeParameterError, "请求参数错误")
		return
	}
	if err := h.service.UpdatePassword(c.Request.Context(), userID, req); err != nil {
		handleError(c, err, response.CodeUpdateUserPasswordFailed, "修改密码失败")
		return
	}
	response.Success(c, nil)
}

func currentUserID(c *gin.Context) (int64, bool) {
	value, exists := c.Get(middleware.UserIDKey)
	userID, ok := value.(int64)
	if !exists || !ok || userID <= 0 {
		response.Fail(c, http.StatusInternalServerError, response.CodeTokenUserInvalid, "登录用户信息无效")
		return 0, false
	}
	return userID, true
}

func userResponse(user *model.User) response.UserInfoResponse {
	return response.UserInfoResponse{
		ID:          user.ID,
		Username:    user.Username,
		Nickname:    user.Nickname,
		Status:      user.Status,
		LastLoginAt: user.LastLoginAt,
		Roles:       user.Roles,
	}
}
