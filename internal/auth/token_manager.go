package auth

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrAccessTokenInvalid   = errors.New("invalid access token")
	ErrJWTSecretTooShort    = errors.New("jwt secret must be at least 32 characters")
	ErrJWTIssuerEmpty       = errors.New("jwt issuer is empty")
	ErrJWTExpireInvalid     = errors.New("jwt expiry must be positive")
	ErrTokenUserInvalid     = errors.New("jwt user is invalid")
	ErrTokenUsernameInvalid = errors.New("jwt username is invalid")
)

type contextKey struct{}

type Identity struct {
	UserID   int64
	Username string
}

type UserClaims struct {
	Username string `json:"username"`
	UserID   int64  `json:"user_id"`
	jwt.RegisteredClaims
}

type TokenManager struct {
	secret []byte
	issuer string
	ttl    time.Duration
	now    func() time.Time
}

func NewTokenManager(secret, issuer string, ttl time.Duration) (*TokenManager, error) {
	secret = strings.TrimSpace(secret)
	if len(secret) < 32 {
		return nil, ErrJWTSecretTooShort
	}
	if strings.TrimSpace(issuer) == "" {
		return nil, ErrJWTIssuerEmpty
	}
	if ttl <= 0 {
		return nil, ErrJWTExpireInvalid
	}
	return &TokenManager{secret: []byte(secret), issuer: issuer, ttl: ttl, now: time.Now}, nil
}

func (m *TokenManager) GenerateAccessToken(userID int64, username string) (string, error) {
	if userID <= 0 {
		return "", ErrTokenUserInvalid
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return "", ErrTokenUsernameInvalid
	}
	now := m.now()
	claims := UserClaims{
		UserID:   userID,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   strconv.FormatInt(userID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
}

func (m *TokenManager) ParseAccessToken(tokenString string) (*UserClaims, error) {
	claims := &UserClaims{}
	token, err := jwt.ParseWithClaims(
		tokenString,
		claims,
		func(*jwt.Token) (interface{}, error) { return m.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(m.issuer),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithLeeway(30*time.Second),
	)
	if err != nil {
		return nil, err
	}
	if !token.Valid || claims.ExpiresAt == nil || claims.IssuedAt == nil {
		return nil, ErrAccessTokenInvalid
	}
	if claims.UserID <= 0 {
		return nil, ErrTokenUserInvalid
	}
	if strings.TrimSpace(claims.Username) == "" {
		return nil, ErrTokenUsernameInvalid
	}
	return claims, nil
}

func ContextWithIdentity(ctx context.Context, identity Identity) context.Context {
	return context.WithValue(ctx, contextKey{}, identity)
}

func IdentityFromContext(ctx context.Context) (Identity, bool) {
	identity, ok := ctx.Value(contextKey{}).(Identity)
	return identity, ok && identity.UserID > 0
}
