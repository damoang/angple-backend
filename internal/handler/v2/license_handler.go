package v2

import (
	"net/http"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/gin-gonic/gin"
)

// LicenseHandler handles license verification API endpoints
type LicenseHandler struct{}

// NewLicenseHandler creates a new LicenseHandler
func NewLicenseHandler() *LicenseHandler {
	return &LicenseHandler{}
}

type licenseVerifyRequest struct {
	LicenseKey string `json:"license_key" binding:"required"`
	PluginID   string `json:"plugin_id" binding:"required"`
	Domain     string `json:"domain"`
}

// Verify handles POST /api/v1/licenses/verify
// Validates a plugin/theme license key
// TODO: v2 마이그레이션 - DB 재설계 후 라이선스 테이블 연동
func (h *LicenseHandler) Verify(c *gin.Context) {
	var req licenseVerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 요청입니다", err)
		return
	}

	// TODO: 라이선스 테이블 조회 및 검증 로직 구현
	// 현재는 모든 라이선스를 유효로 처리 (오픈소스 단계)
	common.V2Success(c, gin.H{
		"valid":     true,
		"plugin_id": req.PluginID,
		"message":   "라이선스가 유효합니다",
	})
}
