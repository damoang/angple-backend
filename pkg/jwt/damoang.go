package jwt

import (
	"errors"

	"github.com/golang-jwt/jwt/v5"
)

// DamoangClaims - damoang.net JWT 페이로드 구조
// damoang.net에서 생성된 JWT는 다른 필드명을 사용함
// 개발 편의를 위해 두 가지 클레임 형식을 모두 지원:
// - damoang.net 형식: mb_id, mb_name, mb_email, mb_level
// - angple-backend 형식: user_id, nickname, level
type DamoangClaims struct {
	jwt.RegisteredClaims
	// damoang.net 형식
	MbID    string `json:"mb_id"`
	MbName  string `json:"mb_name"`
	MbEmail string `json:"mb_email"`
	MbLevel int    `json:"mb_level"`
	// angple-backend 형식 (개발/테스트용)
	UserID   string `json:"user_id,omitempty"`
	Nickname string `json:"nickname,omitempty"`
	Level    int    `json:"level,omitempty"`
}

// GetUserID returns the user ID, checking both formats
func (c *DamoangClaims) GetUserID() string {
	if c.MbID != "" {
		return c.MbID
	}
	return c.UserID
}

// GetUserLevel returns the user level, checking both formats
func (c *DamoangClaims) GetUserLevel() int {
	if c.MbLevel != 0 {
		return c.MbLevel
	}
	return c.Level
}

// GetUserName returns the user name, checking both formats
func (c *DamoangClaims) GetUserName() string {
	if c.MbName != "" {
		return c.MbName
	}
	return c.Nickname
}

// VerifyDamoangToken - damoang.net에서 생성된 JWT 토큰 검증
// damoang_jwt 쿠키에서 읽은 토큰을 검증할 때 사용
//
//nolint:dupl // JWT 검증 로직은 표준 패턴을 따르므로 유사함
func (m *Manager) VerifyDamoangToken(tokenString string) (*DamoangClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &DamoangClaims{}, func(token *jwt.Token) (interface{}, error) {
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

	if claims, ok := token.Claims.(*DamoangClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, ErrInvalidToken
}

// DamoangManager - damoang.net JWT 전용 매니저
// 별도의 비밀키를 사용할 수 있음
type DamoangManager struct {
	secretKey []byte
}

// NewDamoangManager - damoang.net JWT 매니저 생성
func NewDamoangManager(secret string) *DamoangManager {
	return &DamoangManager{
		secretKey: []byte(secret),
	}
}

// VerifyToken - damoang.net JWT 토큰 검증
//
//nolint:dupl // JWT 검증 로직은 표준 패턴을 따르므로 유사함
func (m *DamoangManager) VerifyToken(tokenString string) (*DamoangClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &DamoangClaims{}, func(token *jwt.Token) (interface{}, error) {
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

	if claims, ok := token.Claims.(*DamoangClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, ErrInvalidToken
}
