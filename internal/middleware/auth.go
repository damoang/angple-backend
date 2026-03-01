package middleware

import (
	"log"
	"net"
	"strconv"
	"strings"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/pkg/jwt"
	"github.com/gin-gonic/gin"
)

// getRemoteIP extracts the actual connection IP from RemoteAddr (ignores X-Forwarded-For)
// RemoteAddr format: "IP:port" or "[IPv6]:port"
func getRemoteIP(c *gin.Context) string {
	remoteAddr := c.Request.RemoteAddr
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		// RemoteAddr에 포트가 없는 경우 그대로 반환
		return remoteAddr
	}
	return host
}

// JWTAuth JWT authentication middleware (Bearer token or Cookie)
// 내부 신뢰 요청 (SvelteKit SSR) 또는 JWT 토큰으로 인증
func JWTAuth(jwtManager *jwt.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 0. 내부 신뢰 요청 확인 (SvelteKit → Backend, 127.0.0.1에서만 유효)
		// 세션 인증을 거친 SvelteKit이 전달하는 사용자 정보
		// 주의: ClientIP()는 X-Forwarded-For를 사용하므로 실제 연결 IP를 확인해야 함
		remoteIP := getRemoteIP(c)
		internalAuth := c.GetHeader("X-Internal-Auth")
		internalSecret := c.GetHeader("X-Internal-Secret")
		internalUserID := c.GetHeader("X-Internal-User-ID")

		// 디버그: admin 요청 로깅
		if strings.Contains(c.Request.URL.Path, "admin") {
			log.Printf("[JWTAuth DEBUG] %s %s | remoteIP=%s clientIP=%s | internalAuth=%s hasSecret=%v userID=%s | hasAuthHeader=%v",
				c.Request.Method, c.Request.URL.Path,
				remoteIP, c.ClientIP(),
				internalAuth, internalSecret != "", internalUserID,
				c.GetHeader("Authorization") != "")
		}

		// 내부 신뢰 요청 확인 (두 가지 방식)
		// 1. 127.0.0.1에서 오는 요청 (nginx → SvelteKit → Backend)
		// 2. 공유 시크릿 일치 (CloudFront가 직접 Backend로 라우팅하는 경우)
		isLocalhost := remoteIP == "127.0.0.1" || remoteIP == "::1"
		hasValidSecret := internalSecret == "angple-internal-dev-2026"
		if internalAuth == "sveltekit-session" && (isLocalhost || hasValidSecret) {
			userID := c.GetHeader("X-Internal-User-ID")
			levelStr := c.GetHeader("X-Internal-User-Level")
			if userID != "" {
				level := 0
				if levelStr != "" {
					if l, err := strconv.Atoi(levelStr); err == nil {
						level = l
					}
				}
				c.Set("userID", userID)
				c.Set("nickname", "")
				c.Set("level", level)
				c.Set("v2_user_id", userID)
				c.Next()
				return
			}
		}

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
