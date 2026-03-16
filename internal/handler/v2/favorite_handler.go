package v2

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// FavoriteHandler handles board favorites (bookmark slots) endpoints
type FavoriteHandler struct {
	db *gorm.DB
}

// NewFavoriteHandler creates a new favorite handler
func NewFavoriteHandler(db *gorm.DB) *FavoriteHandler {
	return &FavoriteHandler{db: db}
}

// FavoriteEntry represents a single board favorite slot
type FavoriteEntry struct {
	BoardID string `json:"boardId"`
	Title   string `json:"title"`
}

// GetFavorites handles GET /api/v1/my/favorites
func (h *FavoriteHandler) GetFavorites(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == "" {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "인증이 필요합니다", nil)
		return
	}

	key := fmt.Sprintf("favorites:%s", userID)

	var result struct {
		ValueText string `gorm:"column:value_text"`
	}
	err := h.db.Raw("SELECT value_text FROM g5_kv_store WHERE `key` = ? LIMIT 1", key).Scan(&result).Error
	if err != nil || result.ValueText == "" {
		// 데이터 없음 — 빈 객체 반환
		common.V2Success(c, map[string]FavoriteEntry{})
		return
	}

	var favorites map[string]FavoriteEntry
	if err := json.Unmarshal([]byte(result.ValueText), &favorites); err != nil {
		common.V2Success(c, map[string]FavoriteEntry{})
		return
	}

	common.V2Success(c, favorites)
}

// UpdateFavorites handles PUT /api/v1/my/favorites
func (h *FavoriteHandler) UpdateFavorites(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == "" {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "인증이 필요합니다", nil)
		return
	}

	var favorites map[string]FavoriteEntry
	if err := c.ShouldBindJSON(&favorites); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "유효하지 않은 요청입니다", err)
		return
	}

	jsonBytes, err := json.Marshal(favorites)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "데이터 직렬화 실패", err)
		return
	}

	key := fmt.Sprintf("favorites:%s", userID)
	now := int(time.Now().Unix())

	result := h.db.Exec(`
		INSERT INTO g5_kv_store (`+"`key`"+`, value_type, value_text, updated_at)
		VALUES (?, 'TEXT', ?, ?)
		ON DUPLICATE KEY UPDATE value_text = VALUES(value_text), updated_at = VALUES(updated_at)
	`, key, string(jsonBytes), now)
	if result.Error != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "즐겨찾기 저장 실패", result.Error)
		return
	}

	common.V2Success(c, gin.H{"success": true})
}
