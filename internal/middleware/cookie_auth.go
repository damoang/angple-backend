package middleware

import (
	"github.com/damoang/angple-backend/pkg/jwt"
	"github.com/gofiber/fiber/v2"
)

// DamoangCookieAuth - damoang_jwt 쿠키에서 인증 정보 추출
// 인증 실패해도 요청을 계속 진행 (optional auth)
func DamoangCookieAuth(damoangJWT *jwt.DamoangManager) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// 1. damoang_jwt 쿠키 읽기
		tokenString := c.Cookies("damoang_jwt")
		if tokenString == "" {
			// 쿠키가 없으면 비로그인 상태로 진행
			return c.Next()
		}

		// 2. JWT 검증
		claims, err := damoangJWT.VerifyToken(tokenString)
		if err != nil {
			// 토큰이 유효하지 않으면 비로그인 상태로 진행
			// 에러 로깅은 필요시 추가
			return c.Next()
		}

		// 3. c.Locals()에 사용자 정보 저장
		c.Locals("damoang_user_id", claims.MbID)
		c.Locals("damoang_user_name", claims.MbName)
		c.Locals("damoang_user_level", claims.MbLevel)
		c.Locals("damoang_user_email", claims.MbEmail)
		c.Locals("damoang_authenticated", true)

		return c.Next()
	}
}

// GetDamoangUserID extracts damoang user ID from context
func GetDamoangUserID(c *fiber.Ctx) string {
	userID, ok := c.Locals("damoang_user_id").(string)
	if !ok {
		return ""
	}
	return userID
}

// GetDamoangUserName extracts damoang user name from context
func GetDamoangUserName(c *fiber.Ctx) string {
	userName, ok := c.Locals("damoang_user_name").(string)
	if !ok {
		return ""
	}
	return userName
}

// GetDamoangUserLevel extracts damoang user level from context
func GetDamoangUserLevel(c *fiber.Ctx) int {
	level, ok := c.Locals("damoang_user_level").(int)
	if !ok {
		return 0
	}
	return level
}

// GetDamoangUserEmail extracts damoang user email from context
func GetDamoangUserEmail(c *fiber.Ctx) string {
	email, ok := c.Locals("damoang_user_email").(string)
	if !ok {
		return ""
	}
	return email
}

// IsDamoangAuthenticated checks if user is authenticated via damoang_jwt
func IsDamoangAuthenticated(c *fiber.Ctx) bool {
	authenticated, ok := c.Locals("damoang_authenticated").(bool)
	if !ok {
		return false
	}
	return authenticated
}
