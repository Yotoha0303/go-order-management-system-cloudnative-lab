package middleware

import (
	"errors"
	"net/http"
	"strings"

	"go-order-management-system/internal/auth"
	"go-order-management-system/internal/response"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

const (
	UserIDKey   = "user_id"
	UsernameKey = "username"
)

func AuthMiddleware(tokenManager *auth.TokenManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		parts := strings.Fields(c.GetHeader("Authorization"))
		if len(parts) == 0 {
			response.Fail(c, http.StatusUnauthorized, response.CodeTokenMissing, "缺少访问令牌")
			c.Abort()
			return
		}
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			response.Fail(c, http.StatusUnauthorized, response.CodeTokenInvalidFormat, "访问令牌格式错误")
			c.Abort()
			return
		}

		claims, err := tokenManager.ParseAccessToken(parts[1])
		if err != nil {
			code, message := response.CodeTokenInvalid, "访问令牌无效"
			switch {
			case errors.Is(err, jwt.ErrTokenMalformed):
				code, message = response.CodeTokenMalformed, "访问令牌格式错误"
			case errors.Is(err, jwt.ErrTokenSignatureInvalid):
				code, message = response.CodeTokenSignatureInvalid, "访问令牌签名无效"
			case errors.Is(err, jwt.ErrTokenExpired):
				code, message = response.CodeTokenExpired, "访问令牌已过期"
			}
			response.Fail(c, http.StatusUnauthorized, code, message)
			c.Abort()
			return
		}

		identity := auth.Identity{UserID: claims.UserID, Username: claims.Username}
		c.Set(UserIDKey, identity.UserID)
		c.Set(UsernameKey, identity.Username)
		c.Request = c.Request.WithContext(auth.ContextWithIdentity(c.Request.Context(), identity))
		c.Next()
	}
}
