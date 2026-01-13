package handler

import (
	"errors"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/config"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/internal/service"
	"github.com/gofiber/fiber/v2"
)

// AuthHandler handles authentication requests
type AuthHandler struct {
	service service.AuthService
	cfg     *config.Config
}

// NewAuthHandler creates a new AuthHandler
func NewAuthHandler(service service.AuthService, cfg *config.Config) *AuthHandler {
	return &AuthHandler{
		service: service,
		cfg:     cfg,
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

// Login godoc
// @Summary      로그인
// @Description  사용자 ID와 비밀번호로 로그인하여 JWT 토큰을 발급받습니다
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        request  body      LoginRequest  true  "로그인 요청"
// @Success      200  {object}  common.APIResponse{data=object{access_token=string,expires_in=int,user=object}}
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /auth/login [post]
func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req LoginRequest
	if err := c.BodyParser(&req); err != nil {
		return common.ErrorResponse(c, 400, "Invalid request body", err)
	}

	// Authenticate
	response, err := h.service.Login(req.UserID, req.Password)
	if errors.Is(err, common.ErrInvalidCredentials) {
		return common.ErrorResponse(c, 401, "Invalid credentials", err)
	}
	if err != nil {
		return common.ErrorResponse(c, 500, "Login failed", err)
	}

	// Set refreshToken as httpOnly cookie
	cookie := &fiber.Cookie{
		Name:     "damoang_refresh_token",
		Value:    response.RefreshToken,
		HTTPOnly: true,
		Secure:   !h.cfg.IsDevelopment(), // HTTPS only in production
		SameSite: "Strict",
		MaxAge:   7 * 24 * 60 * 60, // 7 days (seconds)
		Path:     "/",
	}
	c.Cookie(cookie)

	// Return accessToken only (no refreshToken in response body)
	return c.JSON(common.APIResponse{
		Data: map[string]interface{}{
			"access_token": response.AccessToken,
			"expires_in":   900, // 15 minutes (seconds)
			"user":         response.User,
		},
	})
}

// RefreshToken godoc
// @Summary      토큰 갱신
// @Description  RefreshToken을 사용하여 새로운 AccessToken을 발급받습니다 (Token Rotation 적용)
// @Tags         auth
// @Accept       json
// @Produce      json
// @Success      200  {object}  common.APIResponse{data=object{access_token=string,expires_in=int}}
// @Failure      401  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /auth/refresh [post]
func (h *AuthHandler) RefreshToken(c *fiber.Ctx) error {
	// Read refreshToken from httpOnly cookie
	refreshToken := c.Cookies("damoang_refresh_token")
	if refreshToken == "" {
		return common.ErrorResponse(c, 401, "REFRESH_TOKEN_MISSING",
			errors.New("refresh token cookie not found"))
	}

	// Refresh tokens (Token Rotation)
	tokens, err := h.service.RefreshToken(refreshToken)
	if errors.Is(err, common.ErrInvalidToken) {
		return common.ErrorResponse(c, 401, "INVALID_REFRESH_TOKEN", err)
	}
	if err != nil {
		return common.ErrorResponse(c, 500, "TOKEN_REFRESH_FAILED", err)
	}

	// Set new refreshToken as httpOnly cookie (Token Rotation)
	cookie := &fiber.Cookie{
		Name:     "damoang_refresh_token",
		Value:    tokens.RefreshToken,
		HTTPOnly: true,
		Secure:   !h.cfg.IsDevelopment(),
		SameSite: "Strict",
		MaxAge:   7 * 24 * 60 * 60, // 7 days
		Path:     "/",
	}
	c.Cookie(cookie)

	// Return only accessToken
	return c.JSON(common.APIResponse{
		Data: map[string]interface{}{
			"access_token": tokens.AccessToken,
			"expires_in":   900, // 15 minutes
		},
	})
}

// Logout godoc
// @Summary      로그아웃
// @Description  로그아웃하여 RefreshToken 쿠키를 삭제합니다
// @Tags         auth
// @Accept       json
// @Produce      json
// @Success      200  {object}  common.APIResponse{data=object{message=string}}
// @Router       /auth/logout [post]
func (h *AuthHandler) Logout(c *fiber.Ctx) error {
	// Clear refreshToken cookie
	cookie := &fiber.Cookie{
		Name:     "damoang_refresh_token",
		Value:    "",
		HTTPOnly: true,
		Secure:   !h.cfg.IsDevelopment(),
		SameSite: "Strict",
		MaxAge:   -1, // Expire immediately
		Path:     "/",
	}
	c.Cookie(cookie)

	return c.JSON(common.APIResponse{
		Data: map[string]string{
			"message": "Logged out successfully",
		},
	})
}

// GetProfile godoc
// @Summary      프로필 조회 (JWT 인증 필요)
// @Description  JWT 토큰을 사용하여 현재 로그인한 사용자의 프로필 정보를 조회합니다
// @Tags         auth
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  common.APIResponse{data=object{user_id=string,nickname=string,level=int}}
// @Failure      401  {object}  common.APIResponse
// @Router       /auth/profile [get]
func (h *AuthHandler) GetProfile(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	nickname := middleware.GetNickname(c)
	level := middleware.GetUserLevel(c)

	return c.JSON(common.APIResponse{
		Data: fiber.Map{
			"user_id":  userID,
			"nickname": nickname,
			"level":    level,
		},
	})
}

// GetCurrentUser godoc
// @Summary      현재 사용자 정보 조회 (다모앙 쿠키 인증)
// @Description  다모앙 JWT 쿠키를 통해 현재 로그인한 사용자 정보를 조회합니다 (별도 JWT 토큰 불필요)
// @Tags         auth
// @Accept       json
// @Produce      json
// @Success      200  {object}  common.APIResponse{data=object{mb_id=string,mb_name=string,mb_level=int,mb_email=string}}
// @Router       /auth/me [get]
func (h *AuthHandler) GetCurrentUser(c *fiber.Ctx) error {
	// Check if user is authenticated via damoang_jwt cookie
	if !middleware.IsDamoangAuthenticated(c) {
		return c.JSON(common.APIResponse{
			Data: nil,
		})
	}

	// Return user info from damoang_jwt cookie
	return c.JSON(common.APIResponse{
		Data: fiber.Map{
			"mb_id":    middleware.GetDamoangUserID(c),
			"mb_name":  middleware.GetDamoangUserName(c),
			"mb_level": middleware.GetDamoangUserLevel(c),
			"mb_email": middleware.GetDamoangUserEmail(c),
		},
	})
}
