package middleware

import (
	"strings"

	"github.com/damoang/angple-backend/internal/config"
	"github.com/damoang/angple-backend/pkg/jwt"
	"github.com/gin-gonic/gin"
)

// DamoangCookieAuth - damoang_jwt 쿠키 또는 Bearer 토큰에서 인증 정보 추출
// 인증 실패해도 요청을 계속 진행 (optional auth)
// 쿠키 → Bearer 토큰 순서로 검증 시도
func DamoangCookieAuth(damoangJWT *jwt.DamoangManager, _ *config.Config, jwtManagers ...*jwt.Manager) gin.HandlerFunc {
	// jwtManager는 optional — 전달되면 Bearer 토큰도 검증
	var jwtMgr *jwt.Manager
	if len(jwtManagers) > 0 {
		jwtMgr = jwtManagers[0]
	}

	return func(c *gin.Context) {
		// 1. damoang_jwt 쿠키 읽기
		tokenString, err := c.Cookie("damoang_jwt")
		if err == nil && tokenString != "" {
			// 쿠키 JWT 검증
			claims, verifyErr := damoangJWT.VerifyToken(tokenString)
			if verifyErr == nil {
				// context에 사용자 정보 저장 (두 가지 형식 모두 지원)
				c.Set("damoang_user_id", claims.GetUserID())
				c.Set("damoang_user_name", claims.GetUserName())
				c.Set("damoang_user_level", claims.GetUserLevel())
				c.Set("damoang_user_email", claims.MbEmail)
				c.Set("damoang_authenticated", true)

				// 표준 키도 설정 (RequireAdmin 등 다른 미들웨어 호환)
				c.Set("userID", claims.GetUserID())
				c.Set("nickname", claims.GetUserName())
				c.Set("level", claims.GetUserLevel())

				c.Next()
				return
			}
		}

		// 2. 쿠키가 없거나 유효하지 않으면 Bearer 토큰 확인 (ops 도구 호환)
		if jwtMgr != nil {
			authHeader := c.GetHeader("Authorization")
			if authHeader != "" {
				parts := strings.Split(authHeader, " ")
				if len(parts) == 2 && parts[0] == "Bearer" {
					claims, verifyErr := jwtMgr.VerifyToken(parts[1])
					if verifyErr == nil {
						c.Set("damoang_user_id", claims.UserID)
						c.Set("damoang_user_name", claims.Nickname)
						c.Set("damoang_user_level", claims.Level)
						c.Set("damoang_authenticated", true)

						c.Set("userID", claims.UserID)
						c.Set("nickname", claims.Nickname)
						c.Set("level", claims.Level)

						c.Next()
						return
					}
				}
			}
		}

		// 인증 없이 비로그인 상태로 진행
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
