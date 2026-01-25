package middleware

import (
	"github.com/damoang/angple-backend/internal/config"
	"github.com/damoang/angple-backend/pkg/jwt"
	"github.com/gin-gonic/gin"
)

// DamoangCookieAuth - damoang_jwt 쿠키에서 인증 정보 추출
// 인증 실패해도 요청을 계속 진행 (optional auth)
// 모든 환경에서 실제 JWT 쿠키 검증 수행
func DamoangCookieAuth(damoangJWT *jwt.DamoangManager, _ *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. damoang_jwt 쿠키 읽기
		tokenString, err := c.Cookie("damoang_jwt")
		if err != nil || tokenString == "" {
			// 쿠키가 없으면 비로그인 상태로 진행
			c.Next()
			return
		}

		// 2. JWT 검증
		claims, err := damoangJWT.VerifyToken(tokenString)
		if err != nil {
			// 토큰이 유효하지 않으면 비로그인 상태로 진행
			c.Next()
			return
		}

		// 3. context에 사용자 정보 저장 (두 가지 형식 모두 지원)
		c.Set("damoang_user_id", claims.GetUserID())
		c.Set("damoang_user_name", claims.GetUserName())
		c.Set("damoang_user_level", claims.GetUserLevel())
		c.Set("damoang_user_email", claims.MbEmail) // 이메일은 damoang 형식만
		c.Set("damoang_authenticated", true)

		c.Next()
	}
}

// GetDamoangUserID extracts damoang user ID from context
func GetDamoangUserID(c *gin.Context) string {
	userID, exists := c.Get("damoang_user_id")
	if !exists {
		return ""
	}
	if str, ok := userID.(string); ok {
		return str
	}
	return ""
}

// GetDamoangUserName extracts damoang user name from context
func GetDamoangUserName(c *gin.Context) string {
	userName, exists := c.Get("damoang_user_name")
	if !exists {
		return ""
	}
	if str, ok := userName.(string); ok {
		return str
	}
	return ""
}

// GetDamoangUserLevel extracts damoang user level from context
func GetDamoangUserLevel(c *gin.Context) int {
	level, exists := c.Get("damoang_user_level")
	if !exists {
		return 0
	}
	if lvl, ok := level.(int); ok {
		return lvl
	}
	return 0
}

// GetDamoangUserEmail extracts damoang user email from context
func GetDamoangUserEmail(c *gin.Context) string {
	email, exists := c.Get("damoang_user_email")
	if !exists {
		return ""
	}
	if str, ok := email.(string); ok {
		return str
	}
	return ""
}

// IsDamoangAuthenticated checks if user is authenticated via damoang_jwt
func IsDamoangAuthenticated(c *gin.Context) bool {
	authenticated, exists := c.Get("damoang_authenticated")
	if !exists {
		return false
	}
	if auth, ok := authenticated.(bool); ok {
		return auth
	}
	return false
}
