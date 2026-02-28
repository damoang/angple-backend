package middleware

import (
	"strings"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/pkg/jwt"
	"github.com/gin-gonic/gin"
)

// JWTAuth JWT authentication middleware (Bearer token or Cookie)
func JWTAuth(jwtManager *jwt.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var token string

		// 1. Authorization 헤더에서 토큰 확인
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			parts := strings.Split(authHeader, " ")
			if len(parts) == 2 && parts[0] == "Bearer" {
				token = parts[1]
			}
		}

		// 2. 헤더에 없으면 쿠키에서 확인 (여러 쿠키 이름 시도)
		if token == "" {
			cookieNames := []string{"access_token", "accessToken", "token"}
			for _, name := range cookieNames {
				if cookieToken, err := c.Cookie(name); err == nil && cookieToken != "" {
					token = cookieToken
					break
				}
			}
		}

		// 3. 토큰이 없으면 401
		if token == "" {
			common.ErrorResponse(c, 401, "로그인이 필요합니다", nil)
			c.Abort()
			return
		}

		// 4. 토큰 검증
		claims, err := jwtManager.VerifyToken(token)
		if err != nil {
			common.ErrorResponse(c, 401, "로그인이 필요합니다", nil)
			c.Abort()
			return
		}

		c.Set("userID", claims.UserID)
		c.Set("nickname", claims.Nickname)
		c.Set("level", claims.Level)
		c.Set("v2_user_id", claims.UserID)

		c.Next()
	}
}

// OptionalJWTAuth attempts to authenticate but continues even if no token is present.
// Invalid or expired tokens are silently ignored (user proceeds as unauthenticated).
func OptionalJWTAuth(jwtManager *jwt.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var token string

		// 1. Authorization 헤더에서 토큰 확인
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			parts := strings.Split(authHeader, " ")
			if len(parts) == 2 && parts[0] == "Bearer" {
				token = parts[1]
			}
		}

		// 2. 헤더에 없으면 쿠키에서 확인
		if token == "" {
			if cookieToken, err := c.Cookie("access_token"); err == nil && cookieToken != "" {
				token = cookieToken
			}
		}

		// 3. 토큰이 있으면 검증
		if token != "" {
			claims, err := jwtManager.VerifyToken(token)
			if err == nil {
				c.Set("userID", claims.UserID)
				c.Set("nickname", claims.Nickname)
				c.Set("level", claims.Level)
				c.Set("v2_user_id", claims.UserID)
			}
			// 토큰 검증 실패 시 무시 (비인증 상태로 계속)
		}

		c.Next()
	}
}

// GetUserID extracts user ID from context
func GetUserID(c *gin.Context) string {
	userID, exists := c.Get("userID")
	if !exists {
		return ""
	}
	if str, ok := userID.(string); ok {
		return str
	}
	return ""
}

// GetUserLevel extracts user level from context
func GetUserLevel(c *gin.Context) int {
	level, exists := c.Get("level")
	if !exists {
		return 0
	}
	if lvl, ok := level.(int); ok {
		return lvl
	}
	return 0
}

// GetNickname extracts nickname from context
func GetNickname(c *gin.Context) string {
	nickname, exists := c.Get("nickname")
	if !exists {
		return ""
	}
	if str, ok := nickname.(string); ok {
		return str
	}
	return ""
}
