package middleware

import (
	"crypto/subtle"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/pkg/jwt"
	"github.com/gin-gonic/gin"
)

// internalSharedSecret 은 SvelteKit SSR 등 내부 호출자가 제시해야 하는 공유 비밀이다.
// 부팅 시 1회만 읽는다 — 요청 헤더에서 읽으면 "자기 자신과 비교"가 되어 인증이 무너진다.
//
// ⛔ 2026-08-08 사고: `internalSecret := c.GetHeader("X-Internal-Secret")` 로 바뀐 뒤
//
//	`internalSecret == c.GetHeader("X-Internal-Secret")` 를 검사해, 헤더가 비어있지만
//	않으면 항상 참이었다. 그 결과 아무나 X-Internal-User-ID/Level 을 붙여 **임의 회원
//	사칭·관리자 승격**이 가능했다(인터넷에서 실증). 원래 os.Getenv 였던 것이
//	lint 정리 커밋에서 지역변수로 회귀한 것이 원인.
var internalSharedSecret = os.Getenv("INTERNAL_SECRET")

// hasValidInternalSecret 은 제시된 헤더가 서버가 보유한 공유 비밀과 일치하는지 본다.
// 비밀이 설정되지 않았으면 이 경로는 **항상 거부**한다(fail-closed).
// 비교는 타이밍 공격을 피해 상수 시간으로 한다.
func hasValidInternalSecret(headerSecret string) bool {
	if internalSharedSecret == "" || headerSecret == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(headerSecret), []byte(internalSharedSecret)) == 1
}

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

		// 내부 신뢰 요청 확인 (두 가지 방식)
		// 1. 127.0.0.1에서 오는 요청 (nginx → SvelteKit → Backend)
		// 2. 공유 시크릿 일치 (CloudFront가 직접 Backend로 라우팅하는 경우)
		isLocalhost := remoteIP == localhostIPv4 || remoteIP == localhostIPv6
		hasValidSecret := hasValidInternalSecret(internalSecret)
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
				c.Set("username", userID) // 내부 신뢰 경로의 X-Internal-User-ID 는 mb_id(username)이다
				c.Set("nickname", "")
				c.Set("level", level)
				c.Set("v2_user_id", userID)
				// 탈퇴 게이트 — ⛔ 이 분기를 빠뜨리면 우회로가 된다(JWT 검증을 건너뛰는 경로).
				//    여기서는 X-Internal-User-ID 가 곧 mb_id 다.
				if blockIfWithdrawn(c, userID) {
					return
				}
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
		c.Set("username", claims.Username) // Bearer 경로: userID 는 v2_users.id(숫자), username 은 mb_id
		c.Set("nickname", claims.Nickname)
		c.Set("level", claims.Level)
		c.Set("v2_user_id", claims.UserID)

		// 탈퇴 게이트 — ⛔ 판정 키는 username(mb_id) 이다.
		//    claims.UserID 는 v2_users.id(숫자)라 g5_member 조회에 쓸 수 없다.
		if blockIfWithdrawn(c, claims.Username) {
			return
		}

		c.Next()
	}
}

// OptionalJWTAuth attempts to authenticate but continues even if no token is present.
// Invalid or expired tokens are silently ignored (user proceeds as unauthenticated).
func OptionalJWTAuth(jwtManager *jwt.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 0. 내부 신뢰 요청 확인 (SvelteKit → Backend)
		remoteIP := getRemoteIP(c)
		internalAuth := c.GetHeader("X-Internal-Auth")
		internalSecret := c.GetHeader("X-Internal-Secret")
		isLocalhost := remoteIP == localhostIPv4 || remoteIP == localhostIPv6
		hasValidSecret := hasValidInternalSecret(internalSecret)
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
				c.Set("username", userID)
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
				c.Set("username", claims.Username)
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

// GetUsername extracts the member username (g5_member.mb_id) from context.
// Bearer 토큰 경로에서 GetUserID 는 v2_users.id(숫자)를 반환하므로, g5_member 접근에는
// 반드시 이 username(mb_id)을 사용해야 한다.
func GetUsername(c *gin.Context) string {
	username, exists := c.Get("username")
	if !exists {
		return ""
	}
	if str, ok := username.(string); ok {
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

// RemapUserIDToMbID 는 Bearer(v2) 토큰 경로에서 컨텍스트 "userID"(=v2_users.id 숫자)를
// "username"(=g5_member.mb_id)으로 덮어써, GetUserID(c) 가 mb_id 를 반환하게 한다.
// gnuboard g5_write_* 쓰기 핸들러(mb_id 기준으로 작성자·포인트·권한 처리)를 v2 앱 라우트에서
// 그대로 재사용하기 위한 어댑터. JWTAuth 이후에 배치해야 한다.
func RemapUserIDToMbID() gin.HandlerFunc {
	return func(c *gin.Context) {
		if username, exists := c.Get("username"); exists {
			if s, ok := username.(string); ok && s != "" {
				c.Set("userID", s)
			}
		}
		c.Next()
	}
}
