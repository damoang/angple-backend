// 나눔 플러그인 관리자 핸들러
package handlers

import (
	"net/http"
	"strconv"

	"github.com/damoang/angple-backend/plugins/giving/service"

	"github.com/gin-gonic/gin"
)

// AdminHandler 나눔 관리자 API 핸들러
type AdminHandler struct {
	svc service.GivingService
}

// NewAdminHandler 관리자 핸들러 생성자
func NewAdminHandler(svc service.GivingService) *AdminHandler {
	return &AdminHandler{svc: svc}
}

// PauseGiving 나눔 일시정지
// POST /api/plugins/giving/admin/:id/pause
func (h *AdminHandler) PauseGiving(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid post ID",
		})
		return
	}

	if err := h.svc.PauseGiving(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "나눔이 일시정지되었습니다.",
	})
}

// ResumeGiving 나눔 재개
// POST /api/plugins/giving/admin/:id/resume
func (h *AdminHandler) ResumeGiving(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid post ID",
		})
		return
	}

	if err := h.svc.ResumeGiving(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "나눔이 재개되었습니다.",
	})
}

// ForceStopGiving 나눔 강제종료
// POST /api/plugins/giving/admin/:id/force-stop
func (h *AdminHandler) ForceStopGiving(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid post ID",
		})
		return
	}

	if err := h.svc.ForceStopGiving(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "나눔이 강제종료되었습니다.",
	})
}

// GetAdminStats 나눔 관리자 통계
// GET /api/plugins/giving/admin/:id/stats
func (h *AdminHandler) GetAdminStats(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid post ID",
		})
		return
	}

	stats, err := h.svc.GetAdminStats(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to fetch admin stats",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    stats,
	})
}
