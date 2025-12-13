package handler

import (
	"errors"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/internal/service"
	"github.com/gofiber/fiber/v2"
)

// AuthHandler handles authentication requests
type AuthHandler struct {
	service service.AuthService
}

// NewAuthHandler creates a new AuthHandler
func NewAuthHandler(service service.AuthService) *AuthHandler {
	return &AuthHandler{service: service}
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

	return c.JSON(common.APIResponse{Data: response})
}

// RefreshToken handles POST /api/v2/auth/refresh
func (h *AuthHandler) RefreshToken(c *fiber.Ctx) error {
	var req RefreshRequest
	if err := c.BodyParser(&req); err != nil {
		return common.ErrorResponse(c, 400, "Invalid request body", err)
	}

	// Refresh tokens
	tokens, err := h.service.RefreshToken(req.RefreshToken)
	if errors.Is(err, common.ErrInvalidToken) {
		return common.ErrorResponse(c, 401, "Invalid refresh token", err)
	}
	if err != nil {
		return common.ErrorResponse(c, 500, "Token refresh failed", err)
	}

	return c.JSON(common.APIResponse{Data: tokens})
}

// GetProfile handles GET /api/v2/auth/profile (requires JWT)
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

// GetCurrentUser handles GET /api/v2/auth/me
// Returns current user info from damoang_jwt cookie (no JWT required)
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
