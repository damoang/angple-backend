package middleware

import (
	"strings"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/pkg/jwt"
	"github.com/gin-gonic/gin"
)

// JWTAuth JWT authentication middleware
// Supports both Authorization header (Bearer token) and damoang_jwt cookie authentication
func JWTAuth(jwtManager *jwt.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. Check Authorization header first
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			// Parse Bearer token
			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || parts[0] != "Bearer" {
				common.ErrorResponse(c, 401, "로그인이 필요합니다", nil)
				c.Abort()
				return
			}

			tokenString := parts[1]

			// Verify token
			claims, err := jwtManager.VerifyToken(tokenString)
			if err != nil {
				common.ErrorResponse(c, 401, "로그인이 필요합니다", nil)
				c.Abort()
				return
			}

			// Store user info in context
			c.Set("userID", claims.UserID)
			c.Set("nickname", claims.Nickname)
			c.Set("level", claims.Level)

			// Bearer 토큰의 UserID는 이미 v2_users.id
			c.Set("v2_user_id", claims.UserID)

			c.Next()
			return
		}

		// 2. Fallback: Check damoang_jwt cookie authentication (set by DamoangCookieAuth middleware)
		if IsDamoangAuthenticated(c) {
			// Copy damoang context values to standard context keys
			c.Set("userID", GetDamoangUserID(c))
			c.Set("nickname", GetDamoangUserName(c))
			c.Set("level", GetDamoangUserLevel(c))

			// v2_user_id는 DamoangCookieAuth에서 이미 설정됨 (있는 경우)

			c.Next()
			return
		}

		// 3. No valid authentication found
		common.ErrorResponse(c, 401, "로그인이 필요합니다", nil)
		c.Abort()
	}
}

// OptionalJWTAuth attempts to authenticate but continues even if no token is present
// Supports both Authorization header (Bearer token) and damoang_jwt cookie authentication
func OptionalJWTAuth(jwtManager *jwt.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. Check Authorization header first
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || parts[0] != "Bearer" {
				common.ErrorResponse(c, 401, "로그인이 필요합니다", nil)
				c.Abort()
				return
			}

			tokenString := parts[1]
			claims, err := jwtManager.VerifyToken(tokenString)
			if err != nil {
				common.ErrorResponse(c, 401, "로그인이 필요합니다", nil)
				c.Abort()
				return
			}

			c.Set("userID", claims.UserID)
			c.Set("nickname", claims.Nickname)
			c.Set("level", claims.Level)

			// Bearer 토큰의 UserID는 이미 v2_users.id
			c.Set("v2_user_id", claims.UserID)

			c.Next()
			return
		}

		// 2. Fallback: Check damoang_jwt cookie authentication
		if IsDamoangAuthenticated(c) {
			c.Set("userID", GetDamoangUserID(c))
			c.Set("nickname", GetDamoangUserName(c))
			c.Set("level", GetDamoangUserLevel(c))
			// v2_user_id는 DamoangCookieAuth에서 이미 설정됨 (있는 경우)
		}

		// Continue even without authentication (optional)
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
