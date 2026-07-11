package internalapi

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const Header = "X-Internal-Token"

func Middleware(expected string) gin.HandlerFunc {
	expected = strings.TrimSpace(expected)
	return func(c *gin.Context) {
		actual := c.GetHeader(Header)
		if expected == "" || len(actual) != len(expected) || subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code": 40101,
				"msg":  "invalid internal service token",
			})
			return
		}
		c.Next()
	}
}

func Set(req *http.Request, token string) {
	if req == nil {
		return
	}
	req.Header.Set(Header, token)
}
