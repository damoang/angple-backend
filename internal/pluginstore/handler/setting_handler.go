package handler

import (
	"fmt"
	"net/http"

	"github.com/damoang/angple-backend/internal/plugin"
	"github.com/damoang/angple-backend/internal/pluginstore/service"
	"github.com/gin-gonic/gin"
)

// SettingHandler 플러그인 설정 핸들러
type SettingHandler struct {
	settingSvc *service.SettingService
	reloader   plugin.PluginReloader
}

// NewSettingHandler 생성자
func NewSettingHandler(settingSvc *service.SettingService, reloader plugin.PluginReloader) *SettingHandler {
	return &SettingHandler{settingSvc: settingSvc, reloader: reloader}
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

	var raw map[string]interface{}
	if err := c.ShouldBindJSON(&raw); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{"code": "INVALID_REQUEST", "message": "잘못된 요청 형식입니다"},
		})
		return
	}

	// interface{} → string 변환 (validator는 string 기반)
	req := make(map[string]string, len(raw))
	for k, v := range raw {
		req[k] = fmt.Sprintf("%v", v)
	}

	if err := h.settingSvc.SaveSettings(name, req, actorID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{"code": "SAVE_ERROR", "message": err.Error()},
		})
		return
	}

	// 설정 변경 후 플러그인 재초기화
	if h.reloader != nil {
		if err := h.reloader.ReloadPlugin(name); err != nil {
			c.JSON(http.StatusOK, gin.H{"data": gin.H{
				"message": "설정이 저장되었습니다 (재시작 실패: " + err.Error() + ")",
			}})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{"message": "설정이 저장되었습니다"}})
}

// ExportSettings 플러그인 설정 내보내기
// GET /api/v2/admin/plugins/:name/settings/export
func (h *SettingHandler) ExportSettings(c *gin.Context) {
	name := c.Param("name")

	export, err := h.settingSvc.ExportSettings(name)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{"code": "EXPORT_ERROR", "message": err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": export})
}

// ExportAllSettings 전체 플러그인 설정 내보내기
// GET /api/v2/admin/plugins/settings/export
func (h *SettingHandler) ExportAllSettings(c *gin.Context) {
	exports, err := h.settingSvc.ExportAllSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{"code": "EXPORT_ERROR", "message": err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": exports})
}

// ImportSettings 플러그인 설정 가져오기
// POST /api/v2/admin/plugins/settings/import
func (h *SettingHandler) ImportSettings(c *gin.Context) {
	actorID := getActorID(c)

	var exports []service.PluginConfigExport
	if err := c.ShouldBindJSON(&exports); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{"code": "INVALID_REQUEST", "message": "잘못된 요청 형식입니다"},
		})
		return
	}

	imported, skipped := h.settingSvc.ImportSettings(exports, actorID)

	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"imported": imported,
		"skipped":  skipped,
	}})
}
