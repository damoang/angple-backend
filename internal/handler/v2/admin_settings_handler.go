package v2

import (
	"net/http"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AdminSettingsHandler handles admin settings endpoints
type AdminSettingsHandler struct {
	db *gorm.DB
}

// NewAdminSettingsHandler creates a new admin settings handler
func NewAdminSettingsHandler(db *gorm.DB) *AdminSettingsHandler {
	return &AdminSettingsHandler{db: db}
}

// GetReportLockThreshold handles GET /api/v1/admin/settings/report-lock
func (h *AdminSettingsHandler) GetReportLockThreshold(c *gin.Context) {
	var result struct {
		ValueInt int `gorm:"column:value_int"`
	}
	err := h.db.Raw("SELECT value_int FROM g5_kv_store WHERE `key` = 'system:report_lock_threshold' LIMIT 1").Scan(&result).Error
	if err != nil {
		common.V2Success(c, gin.H{"threshold": 0})
		return
	}
	common.V2Success(c, gin.H{"threshold": result.ValueInt})
}

// UpdateReportLockThreshold handles PUT /api/v1/admin/settings/report-lock
func (h *AdminSettingsHandler) UpdateReportLockThreshold(c *gin.Context) {
	var req struct {
		Threshold int `json:"threshold" binding:"min=0"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "유효하지 않은 요청입니다", err)
		return
	}

	now := int(time.Now().Unix())

	// UPSERT into g5_kv_store
	result := h.db.Exec(`
		INSERT INTO g5_kv_store (`+"`key`"+`, value_type, value_int, updated_at)
		VALUES ('system:report_lock_threshold', 'INT', ?, ?)
		ON DUPLICATE KEY UPDATE value_int = VALUES(value_int), updated_at = VALUES(updated_at)
	`, req.Threshold, now)
	if result.Error != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "설정 저장 실패", result.Error)
		return
	}

	common.V2Success(c, gin.H{"threshold": req.Threshold})
}
