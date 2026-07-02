package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "0123456789abcdef0123456789abcdef"

func TestTokenManagerRoundTrip(t *testing.T) {
	manager, err := NewTokenManager(testSecret, "test-issuer", time.Hour)
	if err != nil {
		t.Fatalf("new token manager: %v", err)
	}
	token, err := manager.GenerateAccessToken(42, "alice")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	claims, err := manager.ParseAccessToken(token)
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}
	if claims.UserID != 42 || claims.Username != "alice" || claims.Subject != "42" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestTokenManagerRejectsExpiredToken(t *testing.T) {
	manager, err := NewTokenManager(testSecret, "test-issuer", time.Hour)
	if err != nil {
		t.Fatalf("new token manager: %v", err)
	}
	manager.now = func() time.Time { return time.Now().Add(-2 * time.Hour) }
	token, err := manager.GenerateAccessToken(1, "alice")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	if _, err := manager.ParseAccessToken(token); !errors.Is(err, jwt.ErrTokenExpired) {
		t.Fatalf("expected expired token error, got %v", err)
	}
}

func TestNewTokenManagerValidatesConfiguration(t *testing.T) {
	if _, err := NewTokenManager("short", "issuer", time.Hour); !errors.Is(err, ErrJWTSecretTooShort) {
		t.Fatalf("expected short secret error, got %v", err)
	}
	if _, err := NewTokenManager(testSecret, "", time.Hour); !errors.Is(err, ErrJWTIssuerEmpty) {
		t.Fatalf("expected issuer error, got %v", err)
	}
}
