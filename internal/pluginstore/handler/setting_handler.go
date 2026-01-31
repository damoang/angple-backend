package handler

import (
	"net/http"

	"github.com/damoang/angple-backend/internal/pluginstore/service"
	"github.com/gin-gonic/gin"
)

// SettingHandler 플러그인 설정 핸들러
type SettingHandler struct {
	settingSvc *service.SettingService
}

// NewSettingHandler 생성자
func NewSettingHandler(settingSvc *service.SettingService) *SettingHandler {
	return &SettingHandler{settingSvc: settingSvc}
}

// GetSettings 플러그인 설정 조회
// GET /api/v2/admin/plugins/:name/settings
func (h *SettingHandler) GetSettings(c *gin.Context) {
	name := c.Param("name")

	settings, err := h.settingSvc.GetSettings(name)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{"code": "SETTINGS_ERROR", "message": err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": settings})
}

// SaveSettings 플러그인 설정 저장
// PUT /api/v2/admin/plugins/:name/settings
func (h *SettingHandler) SaveSettings(c *gin.Context) {
	name := c.Param("name")
	actorID := getActorID(c)

	var req map[string]string
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{"code": "INVALID_REQUEST", "message": "잘못된 요청 형식입니다"},
		})
		return
	}

	if err := h.settingSvc.SaveSettings(name, req, actorID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{"code": "SAVE_ERROR", "message": err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{"message": "설정이 저장되었습니다"}})
}
