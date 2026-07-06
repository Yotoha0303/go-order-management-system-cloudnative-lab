package middleware

import (
	"context"
	"net/http"

	"go-order-management-system/internal/model"
	"go-order-management-system/internal/response"

	"github.com/gin-gonic/gin"
)

type RoleChecker interface {
	HasRole(
		ctx context.Context,
		userID int64,
		roleName string,
	) (bool, error)
}

func AdminMiddleware(roleChecker RoleChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		value, exists := c.Get(UserIDKey)
		userID, ok := value.(int64)
		if !exists || !ok || userID <= 0 {
			response.Fail(c, http.StatusUnauthorized, response.CodeTokenUserInvalid, "登录用户信息无效")
			c.Abort()
			return
		}

		if roleChecker == nil {
			response.Fail(c, http.StatusInternalServerError, response.CodeUserRoleCheckFailed, "权限校验服务未初始化")
			c.Abort()
			return
		}

		allowed, err := roleChecker.HasRole(
			c.Request.Context(),
			userID,
			model.RoleAdmin,
		)

		if err != nil {
			response.Fail(
				c,
				http.StatusInternalServerError,
				response.CodeUserRoleCheckFailed,
				"权限校验失败",
			)
			c.Abort()
			return
		}

		if !allowed {
			response.Fail(
				c,
				http.StatusForbidden,
				response.CodePermissionDenied,
				"无管理员权限",
			)
			c.Abort()
			return
		}
		c.Next()
	}
}
