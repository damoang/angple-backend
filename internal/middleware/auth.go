package middleware

import (
	"errors"
	"strings"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/pkg/jwt"
	"github.com/gofiber/fiber/v2"
)

// JWTAuth JWT authentication middleware
func JWTAuth(jwtManager *jwt.Manager) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// 1. Extract Authorization header
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return common.ErrorResponse(c, 401, "Missing authorization header", nil)
		}

		// 2. Parse Bearer token
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			return common.ErrorResponse(c, 401, "Invalid authorization header format", nil)
		}

		tokenString := parts[1]

		// 3. Verify token
		claims, err := jwtManager.VerifyToken(tokenString)
		if err != nil {
			if errors.Is(err, jwt.ErrExpiredToken) {
				return common.ErrorResponse(c, 401, "Token expired", err)
			}
			return common.ErrorResponse(c, 401, "Invalid token", err)
		}

		// 4. Store user info in context
		c.Locals("userID", claims.UserID)
		c.Locals("nickname", claims.Nickname)
		c.Locals("level", claims.Level)

		return c.Next()
	}
}

// GetUserID extracts user ID from context
func GetUserID(c *fiber.Ctx) string {
	userID, ok := c.Locals("userID").(string)
	if !ok {
		return ""
	}
	return userID
}

// GetUserLevel extracts user level from context
func GetUserLevel(c *fiber.Ctx) int {
	level, ok := c.Locals("level").(int)
	if !ok {
		return 0
	}
	return level
}

// GetNickname extracts nickname from context
func GetNickname(c *fiber.Ctx) string {
	nickname, ok := c.Locals("nickname").(string)
	if !ok {
		return ""
	}
	return nickname
}
