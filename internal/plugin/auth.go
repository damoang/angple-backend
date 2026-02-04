package plugin

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// JWTVerifier JWT 토큰 검증 인터페이스 (순환 의존 방지)
type JWTVerifier interface {
	VerifyAndSetContext(c *gin.Context) error
}

// AuthMiddleware 플러그인 라우트용 인증 미들웨어
// auth: "required" - 인증 필수, "optional" - 인증 선택 (있으면 컨텍스트 설정), "none" - 인증 불필요
func AuthMiddleware(authType string, verifier JWTVerifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		if verifier == nil || authType == "none" || authType == "" {
			c.Next()
			return
		}

		err := verifier.VerifyAndSetContext(c)

		switch authType {
		case "required":
			if err != nil {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"error":   "UNAUTHORIZED",
					"message": "Authentication required",
				})
				return
			}
		case "optional":
			// 인증 실패해도 계속 진행 (로그인 안 한 사용자도 접근 가능)
		}

		c.Next()
	}
}

// DefaultJWTVerifier 기본 JWT 검증기 구현
type DefaultJWTVerifier struct {
	verifyFn func(token string) (userID string, nickname string, level int, err error)
}

// NewDefaultJWTVerifier 생성자
func NewDefaultJWTVerifier(verifyFn func(token string) (string, string, int, error)) *DefaultJWTVerifier {
	return &DefaultJWTVerifier{verifyFn: verifyFn}
}

// VerifyAndSetContext 토큰 검증 후 컨텍스트에 사용자 정보 설정
func (v *DefaultJWTVerifier) VerifyAndSetContext(c *gin.Context) error {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		return ErrNoToken
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		return ErrInvalidToken
	}

	userID, nickname, level, err := v.verifyFn(parts[1])
	if err != nil {
		return err
	}

	c.Set("userID", userID)
	c.Set("nickname", nickname)
	c.Set("level", level)
	return nil
}

// 인증 관련 에러
var (
	ErrNoToken      = &AuthError{Message: "no token provided"}
	ErrInvalidToken = &AuthError{Message: "invalid token format"}
)

// AuthError 인증 에러
type AuthError struct {
	Message string
}

func (e *AuthError) Error() string {
	return e.Message
}
