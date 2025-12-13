package jwt

import (
	"errors"

	"github.com/golang-jwt/jwt/v5"
)

// DamoangClaims - damoang.net JWT 페이로드 구조
// damoang.net에서 생성된 JWT는 다른 필드명을 사용함
type DamoangClaims struct {
	MbID    string `json:"mb_id"`
	MbLevel int    `json:"mb_level"`
	MbName  string `json:"mb_name"`
	MbEmail string `json:"mb_email"`
	jwt.RegisteredClaims
}

// VerifyDamoangToken - damoang.net에서 생성된 JWT 토큰 검증
// damoang_jwt 쿠키에서 읽은 토큰을 검증할 때 사용
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
