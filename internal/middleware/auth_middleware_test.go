package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go-order-management-system/internal/auth"

	"github.com/gin-gonic/gin"
)

func newAuthTestRouter(t *testing.T) (*gin.Engine, *auth.TokenManager) {
	t.Helper()
	manager, err := auth.NewTokenManager("0123456789abcdef0123456789abcdef", "test", time.Hour)
	if err != nil {
		t.Fatalf("new token manager: %v", err)
	}
	router := gin.New()
	router.GET("/protected", AuthMiddleware(manager), func(c *gin.Context) {
		identity, ok := auth.IdentityFromContext(c.Request.Context())
		if !ok {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.JSON(http.StatusOK, gin.H{"user_id": identity.UserID})
	})
	return router, manager
}

func TestAuthMiddlewareRejectsMissingToken(t *testing.T) {
	router, _ := newAuthTestRouter(t)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/protected", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", recorder.Code)
	}
}

func TestAuthMiddlewareAddsIdentity(t *testing.T) {
	router, manager := newAuthTestRouter(t)
	token, err := manager.GenerateAccessToken(7, "alice")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
}
