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

	// refreshToken → httpOnly 쿠키 설정
	h.setRefreshTokenCookie(c, response.RefreshToken)

	// 응답에는 accessToken + user만 반환 (refreshToken 제외)
	c.JSON(http.StatusOK, common.APIResponse{Data: gin.H{
		"access_token": response.AccessToken,
		"user":         response.User,
	}})
}

// RefreshToken handles POST /api/v2/auth/refresh
// 모범사례: Cookie에서 refresh_token 읽기, 새 토큰도 Cookie로 설정
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	// 1순위: httpOnly 쿠키에서 refreshToken 읽기
	refreshToken, err := c.Cookie("refresh_token")
	if err != nil || refreshToken == "" {
		// 2순위: JSON body (하위 호환)
		var req RefreshRequest
		if err := c.ShouldBindJSON(&req); err != nil || req.RefreshToken == "" {
			common.ErrorResponse(c, 401, "Refresh token not found", nil)
			return
		}
		refreshToken = req.RefreshToken
	}

	// 토큰 갱신 (토큰 로테이션)
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

	// 새 refreshToken → httpOnly 쿠키
	h.setRefreshTokenCookie(c, tokens.RefreshToken)

	// 응답에는 accessToken만 반환
	c.JSON(http.StatusOK, common.APIResponse{Data: gin.H{
		"access_token": tokens.AccessToken,
	}})
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
// Includes mb_point and mb_exp from database
func (h *AuthHandler) GetCurrentUser(c *gin.Context) {
	// Check if user is authenticated via damoang_jwt cookie
	if !middleware.IsDamoangAuthenticated(c) {
		c.JSON(http.StatusOK, common.APIResponse{
			Data: nil,
		})
		return
	}

	userID := middleware.GetDamoangUserID(c)

	// Fetch user info from database (includes point, exp)
	userInfo, err := h.service.GetUserInfo(userID)
	if err != nil {
		// Fallback to JWT cookie info if database fetch fails
		c.JSON(http.StatusOK, common.APIResponse{
			Data: gin.H{
				"mb_id":    userID,
				"mb_name":  middleware.GetDamoangUserName(c),
				"mb_level": middleware.GetDamoangUserLevel(c),
				"mb_email": middleware.GetDamoangUserEmail(c),
			},
		})
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{
		Data: userInfo,
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

// Register handles POST /api/v2/auth/register
// @Summary 회원가입
// @Description 새로운 회원 계정을 생성합니다
// @Tags auth
// @Accept json
// @Produce json
// @Param request body service.RegisterRequest true "회원가입 정보"
// @Success 201 {object} common.APIResponse{data=domain.MemberResponse}
// @Failure 400 {object} common.APIResponse
// @Failure 409 {object} common.APIResponse
// @Router /auth/register [post]
func (h *AuthHandler) Register(c *gin.Context) {
	var req service.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다", err)
		return
	}

	member, err := h.service.Register(&req)
	if err != nil {
		if errors.Is(err, common.ErrUserAlreadyExists) {
			common.ErrorResponse(c, http.StatusConflict, "이미 존재하는 회원입니다", err)
			return
		}
		common.ErrorResponse(c, http.StatusBadRequest, err.Error(), err)
		return
	}

	c.JSON(http.StatusCreated, common.APIResponse{Data: member})
}

// Withdraw handles DELETE /api/v2/members/me
// @Summary 회원 탈퇴
// @Description 현재 로그인한 회원의 탈퇴를 처리합니다 (데이터 보존)
// @Tags auth
// @Produce json
// @Success 200 {object} common.APIResponse
// @Failure 401 {object} common.APIResponse
// @Router /members/me [delete]
func (h *AuthHandler) Withdraw(c *gin.Context) {
	userID := middleware.GetDamoangUserID(c)
	if userID == "" {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	if err := h.service.Withdraw(userID); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, err.Error(), err)
		return
	}

	// 쿠키 삭제
	h.clearRefreshTokenCookie(c)

	c.JSON(http.StatusOK, common.APIResponse{Data: gin.H{
		"message": "회원 탈퇴가 완료되었습니다",
	}})
}

// setRefreshTokenCookie refreshToken을 httpOnly 쿠키로 설정
func (h *AuthHandler) setRefreshTokenCookie(c *gin.Context, token string) {
	secure := !h.config.IsDevelopment()
	maxAge := 60 * 60 * 24 * 7 // 7일

	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(
		"refresh_token", // name
		token,           // value
		maxAge,          // maxAge (seconds)
		"/",             // path
		"",              // domain
		secure,          // secure (dev: false, prod: true)
		true,            // httpOnly
	)
}

// clearRefreshTokenCookie refreshToken 쿠키 삭제
func (h *AuthHandler) clearRefreshTokenCookie(c *gin.Context) {
	secure := !h.config.IsDevelopment()

	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(
		"refresh_token",
		"",
		-1,
		"/",
		"",
		secure,
		true,
	)
}
