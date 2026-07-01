package v2

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/middleware"
	v2svc "github.com/damoang/angple-backend/internal/service/v2"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// V2AuthHandler handles v2 authentication endpoints
//
//nolint:revive
type V2AuthHandler struct {
	authService *v2svc.V2AuthService
}

// NewV2AuthHandler creates a new V2AuthHandler
func NewV2AuthHandler(authService *v2svc.V2AuthService) *V2AuthHandler {
	return &V2AuthHandler{authService: authService}
}

type v2LoginRequest struct {
	Username string `json:"username"`
	UserID   string `json:"user_id"`
	Password string `json:"password" binding:"required"`
}

func (r *v2LoginRequest) GetUsername() string {
	if r.Username != "" {
		return r.Username
	}
	return r.UserID
}

type v2RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// isSecureCookie returns true if cookies should have the Secure flag.
// In release mode (production), Secure=true; in debug/local, Secure=false.
func isSecureCookie() bool {
	return gin.Mode() == gin.ReleaseMode
}

// authDomainGroup — auth_domain_groups 테이블 1 row (legacy, Phase 8 Day 6 에서 deprecated 예정)
type authDomainGroup struct {
	Suffix       string `gorm:"primaryKey"`
	CookieDomain string
}

func (authDomainGroup) TableName() string { return "auth_domain_groups" }

// site — sites 테이블 1 row (Phase 8 Day 1, multi-tenant SaaS 통합 식별)
type site struct {
	Slug         string `gorm:"column:slug"`
	PrimaryHost  string `gorm:"column:primary_host"`
	Aliases      string `gorm:"column:aliases;type:json"` // JSON 문자열 (gorm 이 raw 로 가져옴)
	CookieDomain string `gorm:"column:cookie_domain"`
	Status       string `gorm:"column:status"`
}

func (site) TableName() string { return "sites" }

// cookieDomainCache — DB-backed lookup 의 in-memory cache (5분 TTL)
type cookieDomainCache struct {
	mu       sync.RWMutex
	rules    map[string]string // host/suffix → cookie_domain
	loadedAt time.Time
}

var globalCDCache = &cookieDomainCache{rules: map[string]string{}}

const cdCacheTTL = 5 * time.Minute

// StartCookieDomainCacheRefresher — 앱 부팅 시 1회 호출하여 즉시 cache 로드 + 5분마다 background refresh.
// 새 사이트 admin UI 추가 시 5분 안에 자동 적용.
//
// Phase 8 Day 2 — dual-read: sites 테이블 우선, auth_domain_groups fallback.
//   - sites.primary_host + sites.aliases → cookie_domain (활성 상태만)
//   - auth_domain_groups.suffix (sites 미커버 suffix backfill)
//
// Phase 8 Day 6 에서 auth_domain_groups 제거 예정 (sites 단일 source).
func StartCookieDomainCacheRefresher(db *gorm.DB) {
	refresh := func() {
		m := make(map[string]string)

		// 1) sites 테이블 — primary_host + aliases 등록 (활성만)
		var sites []site
		if err := db.Where("status = ?", "active").Find(&sites).Error; err == nil {
			for _, s := range sites {
				m[s.PrimaryHost] = s.CookieDomain
				if s.Aliases != "" && s.Aliases != "null" {
					var aliases []string
					if err := json.Unmarshal([]byte(s.Aliases), &aliases); err == nil {
						for _, a := range aliases {
							m[a] = s.CookieDomain
						}
					}
				}
			}
		}
		// sites 쿼리 실패 시: m 비어있어도 다음 단계 진행 (auth_domain_groups fallback)

		// 2) auth_domain_groups — sites 미커버 suffix 만 backfill (legacy 호환)
		var rules []authDomainGroup
		if err := db.Find(&rules).Error; err == nil {
			for _, r := range rules {
				if _, exists := m[r.Suffix]; !exists {
					m[r.Suffix] = r.CookieDomain
				}
			}
		}

		// 둘 다 실패 + 기존 cache 있으면 유지 (fail-safe)
		if len(m) == 0 {
			return
		}

		globalCDCache.mu.Lock()
		globalCDCache.rules = m
		globalCDCache.loadedAt = time.Now()
		globalCDCache.mu.Unlock()
	}
	refresh() // 즉시 1회
	go func() {
		ticker := time.NewTicker(cdCacheTTL)
		defer ticker.Stop()
		for range ticker.C {
			refresh()
		}
	}()
}

// cookieDomain returns the cookie Domain for the given request host.
// DB 기반 멀티 테넌트 매핑 (auth_domain_groups 테이블) + 5분 in-memory cache.
//   - 미등록 host → "" (host-only, 가장 안전)
//   - 정확 매치 또는 suffix 매치 시 → 해당 cookie_domain
//
// Development (gin debug mode): 항상 "" (host-only).
// 새 SSO 그룹 추가는 admin UI / SQL 로 auth_domain_groups 에 row 추가 (코드 변경 X).
func cookieDomain(host string) string {
	if gin.Mode() != gin.ReleaseMode {
		return ""
	}
	if i := strings.Index(host, ":"); i >= 0 {
		host = host[:i]
	}
	globalCDCache.mu.RLock()
	defer globalCDCache.mu.RUnlock()
	if cd, ok := globalCDCache.rules[host]; ok {
		return cd
	}
	for suffix, cd := range globalCDCache.rules {
		if strings.HasSuffix(host, "."+suffix) {
			return cd
		}
	}
	return "" // host-only fallback
}

// Login handles POST /api/v2/auth/login
func (h *V2AuthHandler) Login(c *gin.Context) {
	var req v2LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 요청입니다", err)
		return
	}

	username := req.GetUsername()
	if username == "" {
		common.V2ErrorResponse(c, http.StatusBadRequest, "아이디를 입력해주세요", nil)
		return
	}
	resp, err := h.authService.Login(username, req.Password)
	if err != nil {
		if errors.Is(err, common.ErrAccountWithdrawn) {
			// 숙려기간 경과 → 확정(익명화)된 계정. 로그인 불가.
			c.JSON(http.StatusForbidden, common.V2Response{
				Success: false,
				Error:   &common.V2Error{Code: "account_withdrawn", Message: "탈퇴 처리된 계정입니다."},
			})
			return
		}
		common.V2ErrorResponse(c, http.StatusUnauthorized, "로그인에 실패했습니다", err)
		return
	}

	// 탈퇴 숙려중: 정상 로그인 성공이 아니라 취소 가능 상태를 반환. 리프레시 쿠키는 설정하지 않는다.
	// access_token 은 취소(DELETE /members/me/leave) 호출용으로만 함께 내려준다.
	if resp.WithdrawalGrace != nil {
		c.JSON(http.StatusOK, common.V2Response{
			Success: true,
			Data: gin.H{
				"status":       "withdrawal_grace",
				"withdrawal":   resp.WithdrawalGrace,
				"access_token": resp.AccessToken,
				"user":         resp.User,
			},
		})
		return
	}

	// Set refresh token as httpOnly cookie (domain shared across subdomains)
	c.SetCookie("refresh_token", resp.RefreshToken, 7*24*3600, "/", cookieDomain(c.Request.Host), isSecureCookie(), true)

	common.V2Success(c, gin.H{
		"access_token": resp.AccessToken,
		"user":         resp.User,
	})
}

// RefreshToken handles POST /api/v2/auth/refresh
func (h *V2AuthHandler) RefreshToken(c *gin.Context) {
	// Try cookie first, then JSON body
	refreshToken, err := c.Cookie("refresh_token")
	if err != nil || refreshToken == "" {
		var req v2RefreshRequest
		if bindErr := c.ShouldBindJSON(&req); bindErr != nil || req.RefreshToken == "" {
			common.V2ErrorResponse(c, http.StatusUnauthorized, "리프레시 토큰이 없습니다", nil)
			return
		}
		refreshToken = req.RefreshToken
	}

	resp, err := h.authService.RefreshToken(refreshToken)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "토큰 갱신에 실패했습니다", err)
		return
	}

	c.SetCookie("refresh_token", resp.RefreshToken, 7*24*3600, "/", cookieDomain(c.Request.Host), isSecureCookie(), true)

	common.V2Success(c, gin.H{
		"access_token": resp.AccessToken,
		"user":         resp.User,
	})
}

// Logout handles POST /api/v2/auth/logout
//
// 자동 재로그인 방지: damoang.net 외 도메인(muzia.net 등)도 동일 backend 를 쓰므로
// 요청 host 기준 cookie 도 함께 만료시킨다. damoang_jwt 는 일부 클라이언트에서
// access_token 폴백으로 읽히므로 함께 삭제.
func (h *V2AuthHandler) Logout(c *gin.Context) {
	secure := isSecureCookie()
	hostDomain := c.Request.Host
	if i := strings.Index(hostDomain, ":"); i >= 0 {
		hostDomain = hostDomain[:i]
	}

	for _, domain := range []string{cookieDomain(c.Request.Host), "", hostDomain} {
		c.SetCookie("refresh_token", "", -1, "/", domain, secure, true)
		c.SetCookie("damoang_jwt", "", -1, "/", domain, secure, false)
		c.SetCookie("oauth_state", "", -1, "/", domain, secure, true)
	}
	common.V2Success(c, gin.H{"message": "로그아웃되었습니다"})
}

// ExchangeToken handles POST /api/v1/auth/exchange
// Exchanges a refresh_token cookie for a new access token (legacy damoang.net SSO flow)
// TODO: v2 마이그레이션 - DB 재설계 후 세션 기반 인증으로 전환
func (h *V2AuthHandler) ExchangeToken(c *gin.Context) {
	refreshToken, err := c.Cookie("refresh_token")
	if err != nil || refreshToken == "" {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "인증 쿠키가 없습니다", nil)
		return
	}

	resp, err := h.authService.RefreshToken(refreshToken)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "토큰 교환에 실패했습니다", err)
		return
	}

	c.SetCookie("refresh_token", resp.RefreshToken, 7*24*3600, "/", cookieDomain(c.Request.Host), isSecureCookie(), true)

	common.V2Success(c, gin.H{
		"access_token": resp.AccessToken,
		"user":         resp.User,
	})
}

// GetMe handles GET /api/v2/auth/me
func (h *V2AuthHandler) GetMe(c *gin.Context) {
	userIDStr := middleware.GetUserID(c)
	userID, err := strconv.ParseUint(userIDStr, 10, 64)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "인증 정보가 올바르지 않습니다", err)
		return
	}

	user, err := h.authService.GetCurrentUser(userID)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusNotFound, "사용자를 찾을 수 없습니다", err)
		return
	}

	// 탈퇴 숙려/확정 분기: 정상 세션이 아니므로 상태 코드로 반환(프론트가 취소 UI 노출 or 로그아웃 처리).
	switch state, deadline := h.authService.CheckWithdrawalStatus(user.Username, time.Now()); state {
	case common.WithdrawalConfirmed:
		c.JSON(http.StatusForbidden, common.V2Response{
			Success: false,
			Error:   &common.V2Error{Code: "account_withdrawn", Message: "탈퇴 처리된 계정입니다."},
		})
		return
	case common.WithdrawalGrace:
		days := int(deadline.Sub(time.Now()).Hours() / 24)
		if days < 0 {
			days = 0
		}
		c.JSON(http.StatusForbidden, common.V2Response{
			Success: false,
			Error: &common.V2Error{
				Code:    "withdrawal_grace",
				Message: "탈퇴 숙려중입니다. 탈퇴를 취소하시겠습니까?",
				Details: gin.H{
					"leave_date":     deadline.AddDate(0, 0, -common.WithdrawalGraceDays).Format("20060102"),
					"deadline":       deadline.Format("2006-01-02"),
					"days_remaining": days,
				},
			},
		})
		return
	}

	common.V2Success(c, user)
}
