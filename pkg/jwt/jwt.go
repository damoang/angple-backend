package jwt

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("expired token")
)

// Claims JWT claims structure
type Claims struct {
	jwt.RegisteredClaims
	UserID   string `json:"user_id"`
	Nickname string `json:"nickname"`
	Level    int    `json:"level"`
}

// Manager JWT token manager
type Manager struct {
	secretKey     []byte
	accessExpiry  time.Duration
	refreshExpiry time.Duration
}

// NewManager creates a new JWT manager
func NewManager(secret string, accessExpiry, refreshExpiry int) *Manager {
	return &Manager{
		secretKey:     []byte(secret),
		accessExpiry:  time.Duration(accessExpiry) * time.Second,
		refreshExpiry: time.Duration(refreshExpiry) * time.Second,
	}
}

// GenerateAccessToken generates an access token
func (m *Manager) GenerateAccessToken(userID, nickname string, level int) (string, error) {
	claims := &Claims{
		UserID:   userID,
		Nickname: nickname,
		Level:    level,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(m.accessExpiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secretKey)
}

// GenerateRefreshToken generates a refresh token
func (m *Manager) GenerateRefreshToken(userID string) (string, error) {
	claims := &Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(m.refreshExpiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secretKey)
}

// VerifyToken verifies and parses a token
//
//nolint:dupl // JWT 검증 로직은 표준 패턴을 따르므로 유사함
func (m *Manager) VerifyToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// Validate signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return m.secretKey, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, ErrInvalidToken
}

// RefreshAccessToken creates a new access token from refresh token
func (m *Manager) RefreshAccessToken(refreshToken string, nickname string, level int) (string, error) {
	claims, err := m.VerifyToken(refreshToken)
	if err != nil {
		return "", err
	}

	// Generate new access token
	return m.GenerateAccessToken(claims.UserID, nickname, level)
}
