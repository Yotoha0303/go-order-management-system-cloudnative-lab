package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"go-order-management-system/internal/model"

	"github.com/gin-gonic/gin"
)

type fakeRoleChecker struct {
	allowed  bool
	err      error
	calls    int
	userID   int64
	roleName string
}

func (f *fakeRoleChecker) HasRole(_ context.Context, userID int64, roleName string) (bool, error) {
	f.calls++
	f.userID = userID
	f.roleName = roleName
	return f.allowed, f.err
}

func TestAdminMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		userID         int64
		setUser        bool
		checker        *fakeRoleChecker
		wantStatus     int
		wantNextCalled bool
		wantCalls      int
	}{
		{
			name:       "missing identity",
			checker:    &fakeRoleChecker{},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "role check failed",
			userID:     12,
			setUser:    true,
			checker:    &fakeRoleChecker{err: errors.New("database unavailable")},
			wantStatus: http.StatusInternalServerError,
			wantCalls:  1,
		},
		{
			name:       "permission denied",
			userID:     12,
			setUser:    true,
			checker:    &fakeRoleChecker{},
			wantStatus: http.StatusForbidden,
			wantCalls:  1,
		},
		{
			name:           "permission granted",
			userID:         12,
			setUser:        true,
			checker:        &fakeRoleChecker{allowed: true},
			wantStatus:     http.StatusNoContent,
			wantNextCalled: true,
			wantCalls:      1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nextCalled := false
			router := gin.New()
			if tt.setUser {
				router.Use(func(c *gin.Context) {
					c.Set(UserIDKey, tt.userID)
					c.Next()
				})
			}
			router.Use(AdminMiddleware(tt.checker))
			router.GET("/admin", func(c *gin.Context) {
				nextCalled = true
				c.Status(http.StatusNoContent)
			})

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/admin", nil))

			if recorder.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d", tt.wantStatus, recorder.Code)
			}
			if nextCalled != tt.wantNextCalled {
				t.Fatalf("expected next called %v, got %v", tt.wantNextCalled, nextCalled)
			}
			if tt.checker.calls != tt.wantCalls {
				t.Fatalf("expected %d role checks, got %d", tt.wantCalls, tt.checker.calls)
			}
			if tt.wantCalls == 1 {
				if tt.checker.userID != tt.userID || tt.checker.roleName != model.RoleAdmin {
					t.Fatalf("unexpected role check: userID=%d role=%q", tt.checker.userID, tt.checker.roleName)
				}
			}
		})
	}
}

func TestAdminMiddlewareRejectsNilRoleChecker(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(UserIDKey, int64(1))
		c.Next()
	})
	router.Use(AdminMiddleware(nil))
	router.GET("/admin", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/admin", nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, recorder.Code)
	}
}
