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

	c.JSON(http.StatusOK, common.APIResponse{Data: response})
}

// RefreshToken handles POST /api/v2/auth/refresh
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	var req RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, 400, "Invalid request body", err)
		return
	}

	// Refresh tokens
	tokens, err := h.service.RefreshToken(req.RefreshToken)
	if errors.Is(err, common.ErrInvalidToken) {
		common.ErrorResponse(c, 401, "Invalid refresh token", err)
		return
	}
	if err != nil {
		common.ErrorResponse(c, 500, "Token refresh failed", err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: tokens})
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
	// Clear the refresh token cookie
	c.SetCookie(
		"refresh_token", // name
		"",              // value
		-1,              // maxAge
		"/",             // path
		"",              // domain
		true,            // secure
		true,            // httpOnly
	)

	c.JSON(http.StatusOK, common.APIResponse{
		Data: gin.H{
			"message": "Logged out successfully",
		},
	})
}
