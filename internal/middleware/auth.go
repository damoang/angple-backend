package middleware

import (
	"errors"
	"strings"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/pkg/jwt"
	"github.com/gin-gonic/gin"
)

// JWTAuth JWT authentication middleware
func JWTAuth(jwtManager *jwt.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. Extract Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			common.ErrorResponse(c, 401, "Missing authorization header", nil)
			c.Abort()
			return
		}

		// 2. Parse Bearer token
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			common.ErrorResponse(c, 401, "Invalid authorization header format", nil)
			c.Abort()
			return
		}

		tokenString := parts[1]

		// 3. Verify token
		claims, err := jwtManager.VerifyToken(tokenString)
		if err != nil {
			if errors.Is(err, jwt.ErrExpiredToken) {
				common.ErrorResponse(c, 401, "Token expired", err)
			} else {
				common.ErrorResponse(c, 401, "Invalid token", err)
			}
			c.Abort()
			return
		}

		// 4. Store user info in context
		c.Set("userID", claims.UserID)
		c.Set("nickname", claims.Nickname)
		c.Set("level", claims.Level)

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
