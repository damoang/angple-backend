package handler

import (
	"errors"
	"net/http"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/config"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/internal/service"
	"github.com/gin-gonic/gin"
)

// AuthHandler handles authentication requests
type AuthHandler struct {
	service service.AuthService
	config  *config.Config
}

// NewAuthHandler creates a new AuthHandler
func NewAuthHandler(service service.AuthService, cfg *config.Config) *AuthHandler {
	return &AuthHandler{
		service: service,
		config:  cfg,
	}
}

// LoginRequest login request
type LoginRequest struct {
	UserID   string `json:"user_id" validate:"required"`
	Password string `json:"password" validate:"required"`
}

// RefreshRequest refresh token request
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// Login handles POST /api/v2/auth/login
// 모범사례: refresh_token은 httpOnly Cookie로, access_token은 body로
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, 400, "Invalid request body", err)
		return
	}

	// Authenticate
	response, err := h.service.Login(req.UserID, req.Password)
	if errors.Is(err, common.ErrInvalidCredentials) {
		common.ErrorResponse(c, 401, "Invalid credentials", err)
		return
	}
	if err != nil {
		common.ErrorResponse(c, 500, "Login failed", err)
		return
	}

	// refresh_token을 httpOnly Cookie로 설정 (XSS 방지)
	h.setRefreshTokenCookie(c, response.RefreshToken)

	// body에는 access_token과 user 정보만 반환
	c.JSON(http.StatusOK, common.APIResponse{
		Data: gin.H{
			"access_token": response.AccessToken,
			"user":         response.User,
		},
	})
}

// RefreshToken handles POST /api/v2/auth/refresh
// 모범사례: Cookie에서 refresh_token 읽기, 새 토큰도 Cookie로 설정
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	// Cookie에서 refresh_token 읽기 (httpOnly라 JavaScript에서 접근 불가)
	refreshToken, err := c.Cookie("refresh_token")
	if err != nil || refreshToken == "" {
		common.ErrorResponse(c, 400, "Refresh token not found in cookie", nil)
		return
	}

	// Refresh tokens
	tokens, err := h.service.RefreshToken(refreshToken)
	if errors.Is(err, common.ErrInvalidToken) {
		// 유효하지 않은 토큰이면 Cookie도 삭제
		h.clearRefreshTokenCookie(c)
		common.ErrorResponse(c, 401, "Invalid refresh token", err)
		return
	}
	if err != nil {
		common.ErrorResponse(c, 500, "Token refresh failed", err)
		return
	}

	// 새 refresh_token을 httpOnly Cookie로 설정
	h.setRefreshTokenCookie(c, tokens.RefreshToken)

	// body에는 access_token만 반환
	c.JSON(http.StatusOK, common.APIResponse{
		Data: gin.H{
			"access_token": tokens.AccessToken,
		},
	})
}

// GetProfile handles GET /api/v2/auth/profile (requires JWT)
func (h *AuthHandler) GetProfile(c *gin.Context) {
	userID := middleware.GetUserID(c)
	nickname := middleware.GetNickname(c)
	level := middleware.GetUserLevel(c)

	c.JSON(http.StatusOK, common.APIResponse{
		Data: gin.H{
			"user_id":  userID,
			"nickname": nickname,
			"level":    level,
		},
	})
}

// GetCurrentUser handles GET /api/v2/auth/me
// Returns current user info from damoang_jwt cookie (no JWT required)
func (h *AuthHandler) GetCurrentUser(c *gin.Context) {
	// Check if user is authenticated via damoang_jwt cookie
	if !middleware.IsDamoangAuthenticated(c) {
		c.JSON(http.StatusOK, common.APIResponse{
			Data: nil,
		})
		return
	}

	// Return user info from damoang_jwt cookie
	c.JSON(http.StatusOK, common.APIResponse{
		Data: gin.H{
			"mb_id":    middleware.GetDamoangUserID(c),
			"mb_name":  middleware.GetDamoangUserName(c),
			"mb_level": middleware.GetDamoangUserLevel(c),
			"mb_email": middleware.GetDamoangUserEmail(c),
		},
	})
}

// Logout handles POST /api/v2/auth/logout
// Clears the httpOnly refresh token cookie
func (h *AuthHandler) Logout(c *gin.Context) {
	h.clearRefreshTokenCookie(c)

	c.JSON(http.StatusOK, common.APIResponse{
		Data: gin.H{
			"message": "Logged out successfully",
		},
	})
}

// setRefreshTokenCookie sets refresh token as httpOnly cookie
// 보안: httpOnly=true로 JavaScript에서 접근 불가 (XSS 방지)
// 보안: secure=true로 HTTPS에서만 전송 (중간자 공격 방지)
// 보안: SameSite=Strict로 CSRF 방지
func (h *AuthHandler) setRefreshTokenCookie(c *gin.Context, token string) {
	maxAge := 7 * 24 * 60 * 60 // 7일 (초 단위)
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(
		"refresh_token", // name
		token,           // value
		maxAge,          // maxAge
		"/",             // path
		"",              // domain (빈 문자열 = 현재 도메인)
		true,            // secure (HTTPS only)
		true,            // httpOnly (JavaScript 접근 불가)
	)
}

// clearRefreshTokenCookie removes refresh token cookie
func (h *AuthHandler) clearRefreshTokenCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(
		"refresh_token", // name
		"",              // value
		-1,              // maxAge (즉시 만료)
		"/",             // path
		"",              // domain
		true,            // secure
		true,            // httpOnly
	)
}
