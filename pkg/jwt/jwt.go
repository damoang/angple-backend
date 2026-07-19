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
	Username string `json:"username,omitempty"`
	Nickname string `json:"nickname"`
	Level    int    `json:"level"`
	Email    string `json:"email,omitempty"`
}

// Manager JWT token manager
type Manager struct {
	secretKey     []byte
	secretKeyNext []byte // 키 롤오버용 보조 검증키(설정 시 주키 실패하면 재시도). 서명엔 미사용.
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

// SetNextKey registers an additional key accepted ONLY for verification (무중단 키 롤오버).
// 서명은 항상 주키(secretKey)를 쓴다. 빈 값이면 비활성 = 기존 동작과 완전히 동일.
func (m *Manager) SetNextKey(next string) {
	if next != "" {
		m.secretKeyNext = []byte(next)
	}
}

// GenerateAccessToken generates an access token
func (m *Manager) GenerateAccessToken(userID, username, nickname string, level int) (string, error) {
	claims := &Claims{
		UserID:   userID,
		Username: username,
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

// VerifyToken verifies and parses a token.
// 주키로 먼저 검증하고, 서명 불일치 등으로 실패하면 보조키(secretKeyNext)가 설정된 경우 재시도한다.
// 만료(ErrExpiredToken)는 재시도하지 않는다. → 무중단 키 롤오버(신·구 키 동시 수용).
func (m *Manager) VerifyToken(tokenString string) (*Claims, error) {
	claims, err := m.verifyWithKey(tokenString, m.secretKey)
	if err != nil && errors.Is(err, ErrInvalidToken) && m.secretKeyNext != nil {
		if c2, e2 := m.verifyWithKey(tokenString, m.secretKeyNext); e2 == nil {
			return c2, nil
		}
	}
	return claims, err
}

//nolint:dupl // JWT 검증 로직은 표준 패턴을 따르므로 유사함
func (m *Manager) verifyWithKey(tokenString string, key []byte) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// Validate signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return key, nil
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
	return m.GenerateAccessToken(claims.UserID, claims.Username, nickname, level)
}
