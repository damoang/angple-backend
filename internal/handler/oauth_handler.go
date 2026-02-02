package handler

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/internal/service"
	"github.com/gin-gonic/gin"
)

// OAuthHandler handles OAuth2 social login endpoints
type OAuthHandler struct {
	oauthService *service.OAuthService
}

// NewOAuthHandler creates a new OAuthHandler
func NewOAuthHandler(oauthService *service.OAuthService) *OAuthHandler {
	return &OAuthHandler{oauthService: oauthService}
}

// Redirect initiates OAuth flow by redirecting to provider's auth page
// GET /api/v2/auth/oauth/:provider
func (h *OAuthHandler) Redirect(c *gin.Context) {
	provider := domain.OAuthProvider(c.Param("provider"))

	// Generate random state for CSRF protection
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to generate state", nil)
		return
	}
	state := hex.EncodeToString(stateBytes)

	// Store state in cookie for verification
	c.SetCookie("oauth_state", state, 600, "/", "", true, true)

	authURL, err := h.oauthService.GetAuthURL(provider, state)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	c.Redirect(http.StatusTemporaryRedirect, authURL)
}

// Callback handles the OAuth provider callback
// GET /api/v2/auth/oauth/:provider/callback
func (h *OAuthHandler) Callback(c *gin.Context) {
	provider := domain.OAuthProvider(c.Param("provider"))

	// Verify state
	state := c.Query("state")
	savedState, err := c.Cookie("oauth_state")
	if err != nil || state == "" || state != savedState {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid OAuth state", nil)
		return
	}

	// Clear state cookie
	c.SetCookie("oauth_state", "", -1, "/", "", true, true)

	code := c.Query("code")
	if code == "" {
		common.ErrorResponse(c, http.StatusBadRequest, "Missing authorization code", nil)
		return
	}

	result, err := h.oauthService.HandleCallback(c.Request.Context(), provider, code)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "OAuth login failed: "+err.Error(), nil)
		return
	}

	common.SuccessResponse(c, result, nil)
}

// GenerateAPIKey creates a new API key for the authenticated user
// POST /api/v2/auth/api-keys
func (h *OAuthHandler) GenerateAPIKey(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == "" {
		common.ErrorResponse(c, http.StatusUnauthorized, "Authentication required", nil)
		return
	}

	var req struct {
		Name   string `json:"name" binding:"required"`
		Scopes string `json:"scopes"` // comma-separated: read,write
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request", nil)
		return
	}

	if req.Scopes == "" {
		req.Scopes = "read"
	}

	apiKey, err := h.oauthService.GenerateAPIKey(c.Request.Context(), userID, req.Name, req.Scopes)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to generate API key", nil)
		return
	}

	c.JSON(http.StatusCreated, gin.H{"success": true, "data": apiKey})
}
