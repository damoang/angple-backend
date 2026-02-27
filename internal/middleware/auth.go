package middleware

import (
	"strings"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/pkg/jwt"
	"github.com/gin-gonic/gin"
)

// JWTAuth JWT authentication middleware (Bearer token only)
func JWTAuth(jwtManager *jwt.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			common.ErrorResponse(c, 401, "로그인이 필요합니다", nil)
			c.Abort()
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			common.ErrorResponse(c, 401, "로그인이 필요합니다", nil)
			c.Abort()
			return
		}

		claims, err := jwtManager.VerifyToken(parts[1])
		if err != nil {
			common.ErrorResponse(c, 401, "로그인이 필요합니다", nil)
			c.Abort()
			return
		}

		userID := claims.UserID
		if userID == "" {
			userID = claims.Subject // SvelteKit JWT 호환 (sub=mb_id)
		}
		c.Set("userID", userID)
		c.Set("nickname", claims.Nickname)
		c.Set("level", claims.Level)
		c.Set("v2_user_id", userID)

		c.Next()
	}
}

// OptionalJWTAuth attempts to authenticate but continues even if no token is present.
// Invalid or expired tokens are silently ignored (user proceeds as unauthenticated).
func OptionalJWTAuth(jwtManager *jwt.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			parts := strings.Split(authHeader, " ")
			if len(parts) == 2 && parts[0] == "Bearer" {
				claims, err := jwtManager.VerifyToken(parts[1])
				if err == nil {
					userID := claims.UserID
					if userID == "" {
						userID = claims.Subject // SvelteKit JWT 호환 (sub=mb_id)
					}
					c.Set("userID", userID)
					c.Set("nickname", claims.Nickname)
					c.Set("level", claims.Level)
					c.Set("v2_user_id", userID)
				}
				// 토큰 검증 실패 시 무시 (비인증 상태로 계속)
			}
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
