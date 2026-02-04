package v2

import (
	"net/http"
	"strconv"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/middleware"
	v2svc "github.com/damoang/angple-backend/internal/service/v2"
	"github.com/gin-gonic/gin"
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
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type v2RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// Login handles POST /api/v2/auth/login
func (h *V2AuthHandler) Login(c *gin.Context) {
	var req v2LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 요청입니다", err)
		return
	}

	resp, err := h.authService.Login(req.Username, req.Password)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "로그인에 실패했습니다", err)
		return
	}

	// Set refresh token as httpOnly cookie
	c.SetCookie("refresh_token", resp.RefreshToken, 7*24*3600, "/", "", true, true)

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

	c.SetCookie("refresh_token", resp.RefreshToken, 7*24*3600, "/", "", true, true)

	common.V2Success(c, gin.H{
		"access_token": resp.AccessToken,
		"user":         resp.User,
	})
}

// Logout handles POST /api/v2/auth/logout
func (h *V2AuthHandler) Logout(c *gin.Context) {
	c.SetCookie("refresh_token", "", -1, "/", "", true, true)
	common.V2Success(c, gin.H{"message": "로그아웃되었습니다"})
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

	common.V2Success(c, user)
}

// ExchangeGnuboardJWT handles POST /api/v2/auth/exchange
// Exchanges damoang_jwt cookie for angple JWT tokens
func (h *V2AuthHandler) ExchangeGnuboardJWT(c *gin.Context) {
	gnuJwt, err := c.Cookie("damoang_jwt")
	if err != nil || gnuJwt == "" {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "damoang_jwt 쿠키가 없습니다", nil)
		return
	}

	resp, err := h.authService.ExchangeGnuboardJWT(gnuJwt)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "토큰 교환 실패", err)
		return
	}

	// Set refresh token as httpOnly cookie
	c.SetCookie("refresh_token", resp.RefreshToken, 7*24*3600, "/", "", true, true)

	common.V2Success(c, gin.H{
		"access_token": resp.AccessToken,
		"user":         resp.User,
	})
}
