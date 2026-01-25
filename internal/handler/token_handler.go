package handler

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/gin-gonic/gin"
)

// TokenHandler handles CSRF token generation
type TokenHandler struct {
	// In-memory token store (should use Redis in production)
	tokens sync.Map
}

// NewTokenHandler creates a new TokenHandler
func NewTokenHandler() *TokenHandler {
	handler := &TokenHandler{}

	// Clean up expired tokens periodically
	go handler.cleanupExpiredTokens()

	return handler
}

// tokenEntry stores token data with expiration
type tokenEntry struct {
	Token     string
	UserID    string
	ExpiresAt time.Time
}

// GenerateWriteTokenRequest request for write token
type GenerateWriteTokenRequest struct {
	TokenCase string `json:"token_case" binding:"required"`
}

// TokenResponse response containing the generated token
type TokenResponse struct {
	Error string `json:"error"`
	Token string `json:"token"`
	URL   string `json:"url"`
}

// GenerateWriteToken handles POST /api/v2/tokens/write
// @Summary 게시글 작성 토큰 생성
// @Description 게시글 작성에 필요한 CSRF 토큰을 생성합니다
// @Tags tokens
// @Accept json
// @Produce json
// @Param request body GenerateWriteTokenRequest true "토큰 케이스"
// @Success 200 {object} common.APIResponse{data=TokenResponse}
// @Failure 400 {object} common.APIResponse
// @Router /tokens/write [post]
func (h *TokenHandler) GenerateWriteToken(c *gin.Context) {
	var req GenerateWriteTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다.", err)
		return
	}

	// Validate token_case (alphanumeric and underscore only)
	if !isValidTokenCase(req.TokenCase) {
		common.ErrorResponse(c, http.StatusBadRequest, "잘못된 토큰 케이스입니다.", nil)
		return
	}

	// Generate token
	token := generateToken()

	// Get user ID if authenticated
	userID := ""
	if middleware.IsDamoangAuthenticated(c) {
		userID = middleware.GetDamoangUserID(c)
	}

	// Store token with expiration (30 minutes)
	key := req.TokenCase + "_" + token
	h.tokens.Store(key, tokenEntry{
		Token:     token,
		UserID:    userID,
		ExpiresAt: time.Now().Add(30 * time.Minute),
	})

	c.JSON(http.StatusOK, common.APIResponse{
		Data: TokenResponse{
			Error: "",
			Token: token,
			URL:   "",
		},
	})
}

// GenerateCommentToken handles POST /api/v2/tokens/comment
// @Summary 댓글 작성 토큰 생성
// @Description 댓글 작성에 필요한 CSRF 토큰을 생성합니다
// @Tags tokens
// @Accept json
// @Produce json
// @Success 200 {object} common.APIResponse{data=TokenResponse}
// @Router /tokens/comment [post]
func (h *TokenHandler) GenerateCommentToken(c *gin.Context) {
	// Generate token
	token := generateToken()

	// Get user ID if authenticated
	userID := ""
	if middleware.IsDamoangAuthenticated(c) {
		userID = middleware.GetDamoangUserID(c)
	}

	// Store token with expiration (30 minutes)
	key := "comment_" + token
	h.tokens.Store(key, tokenEntry{
		Token:     token,
		UserID:    userID,
		ExpiresAt: time.Now().Add(30 * time.Minute),
	})

	c.JSON(http.StatusOK, common.APIResponse{
		Data: TokenResponse{
			Error: "",
			Token: token,
			URL:   "",
		},
	})
}

// ValidateToken validates a token (can be called from other handlers)
func (h *TokenHandler) ValidateToken(tokenCase, token, userID string) bool {
	key := tokenCase + "_" + token
	value, ok := h.tokens.Load(key)
	if !ok {
		return false
	}

	entry, ok := value.(tokenEntry)
	if !ok {
		return false
	}

	// Check expiration
	if time.Now().After(entry.ExpiresAt) {
		h.tokens.Delete(key)
		return false
	}

	// Optionally check user ID
	if userID != "" && entry.UserID != "" && entry.UserID != userID {
		return false
	}

	// Delete token after use (one-time use)
	h.tokens.Delete(key)

	return true
}

// generateToken generates a random token
func generateToken() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp-based token
		return hex.EncodeToString([]byte(time.Now().String()))
	}
	return hex.EncodeToString(bytes)
}

// isValidTokenCase validates token case string
func isValidTokenCase(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return true
}

// cleanupExpiredTokens periodically removes expired tokens
func (h *TokenHandler) cleanupExpiredTokens() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		now := time.Now()
		h.tokens.Range(func(key, value interface{}) bool {
			entry, ok := value.(tokenEntry)
			if !ok {
				return true
			}
			if now.After(entry.ExpiresAt) {
				h.tokens.Delete(key)
			}
			return true
		})
	}
}
