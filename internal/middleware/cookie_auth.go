package middleware

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"

	"github.com/damoang/angple-backend/internal/config"
	v2domain "github.com/damoang/angple-backend/internal/domain/v2"
	"github.com/damoang/angple-backend/pkg/jwt"
	"github.com/gin-gonic/gin"
)

// V2UserFinder — v2_users 테이블에서 username으로 사용자 조회
type V2UserFinder interface {
	FindByUsername(username string) (*v2domain.V2User, error)
}

// cookieAuthConfig holds optional dependencies for DamoangCookieAuth
type cookieAuthConfig struct {
	jwtManager *jwt.Manager
	userRepo   V2UserFinder
}

// CookieAuthOption configures DamoangCookieAuth behavior
type CookieAuthOption func(*cookieAuthConfig)

// WithJWTManager enables Bearer token verification
func WithJWTManager(mgr *jwt.Manager) CookieAuthOption {
	return func(c *cookieAuthConfig) {
		c.jwtManager = mgr
	}
}

// WithV2UserRepo enables mb_id → v2_users.id resolution for cookie auth path
func WithV2UserRepo(repo V2UserFinder) CookieAuthOption {
	return func(c *cookieAuthConfig) {
		c.userRepo = repo
	}
}

// v2UserIDCache — mb_id → v2_users.id 캐시 (프로세스 수명)
var v2UserIDCache sync.Map

// resolveV2UserID resolves mb_id (from cookie) to v2_users.id
func resolveV2UserID(repo V2UserFinder, mbID string) string {
	if mbID == "" {
		return ""
	}
	// 캐시 확인
	if cached, ok := v2UserIDCache.Load(mbID); ok {
		return cached.(string)
	}
	// DB 조회: v2_users.username == mb_id
	user, err := repo.FindByUsername(mbID)
	if err != nil || user == nil {
		log.Printf("[WARN] v2_user_id 조회 실패: mb_id=%s, err=%v", mbID, err)
		return ""
	}
	v2ID := fmt.Sprintf("%d", user.ID)
	v2UserIDCache.Store(mbID, v2ID)
	return v2ID
}

// DamoangCookieAuth - damoang_jwt 쿠키 또는 Bearer 토큰에서 인증 정보 추출
// 인증 실패해도 요청을 계속 진행 (optional auth)
// 쿠키 → Bearer 토큰 순서로 검증 시도
//
// 옵션 패턴 (하위 호환):
//
//	DamoangCookieAuth(damoangJWT, cfg, WithJWTManager(mgr), WithV2UserRepo(repo))
//
// 레거시 호환 (jwtManager를 3번째 인자로 직접 전달):
//
//	DamoangCookieAuth(damoangJWT, cfg, jwtManager)
func DamoangCookieAuth(damoangJWT *jwt.DamoangManager, _ *config.Config, opts ...interface{}) gin.HandlerFunc {
	cfg := &cookieAuthConfig{}

	for _, opt := range opts {
		switch v := opt.(type) {
		case CookieAuthOption:
			v(cfg)
		case *jwt.Manager:
			// 레거시 호환: *jwt.Manager를 직접 전달한 경우
			if v != nil {
				cfg.jwtManager = v
			}
		}
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

				// v2_user_id: mb_id → v2_users.id 변환
				if cfg.userRepo != nil {
					if v2ID := resolveV2UserID(cfg.userRepo, claims.GetUserID()); v2ID != "" {
						c.Set("v2_user_id", v2ID)
					}
				}

				c.Next()
				return
			}
		}

		// 2. 쿠키가 없거나 유효하지 않으면 Bearer 토큰 확인 (ops 도구 호환)
		if cfg.jwtManager != nil {
			authHeader := c.GetHeader("Authorization")
			if authHeader != "" {
				parts := strings.Split(authHeader, " ")
				if len(parts) == 2 && parts[0] == "Bearer" {
					claims, verifyErr := cfg.jwtManager.VerifyToken(parts[1])
					if verifyErr == nil {
						c.Set("damoang_user_id", claims.UserID)
						c.Set("damoang_user_name", claims.Nickname)
						c.Set("damoang_user_level", claims.Level)
						c.Set("damoang_authenticated", true)

						c.Set("userID", claims.UserID)
						c.Set("nickname", claims.Nickname)
						c.Set("level", claims.Level)

						// Bearer 토큰의 UserID는 이미 v2_users.id
						c.Set("v2_user_id", claims.UserID)

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

// GetV2UserID extracts v2_users.id from context (정규 사용자 식별자)
func GetV2UserID(c *gin.Context) string {
	if id, ok := c.Get("v2_user_id"); ok {
		if str, ok := id.(string); ok {
			return str
		}
		// uint64 등 숫자 타입 대응
		if num, ok := id.(uint64); ok {
			return strconv.FormatUint(num, 10)
		}
	}
	return ""
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
