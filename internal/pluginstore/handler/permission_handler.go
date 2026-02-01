package handler

import (
	"net/http"

	"github.com/damoang/angple-backend/internal/pluginstore/service"
	"github.com/gin-gonic/gin"
)

// PermissionHandler 플러그인 권한 핸들러
type PermissionHandler struct {
	permSvc *service.PermissionService
}

// NewPermissionHandler 생성자
func NewPermissionHandler(permSvc *service.PermissionService) *PermissionHandler {
	return &PermissionHandler{permSvc: permSvc}
}

// GetPermissions 플러그인 권한 목록 조회
// GET /api/v2/admin/plugins/:name/permissions
func (h *PermissionHandler) GetPermissions(c *gin.Context) {
	name := c.Param("name")

	perms, err := h.permSvc.GetPermissions(name)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{"code": "PERMISSION_ERROR", "message": err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": perms})
}

// UpdatePermission 권한 최소 레벨 변경
// PUT /api/v2/admin/plugins/:name/permissions/:permId
func (h *PermissionHandler) UpdatePermission(c *gin.Context) {
	name := c.Param("name")
	permID := c.Param("permId")

	var req struct {
		MinLevel int `json:"min_level" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{"code": "INVALID_REQUEST", "message": "min_level은 필수입니다"},
		})
		return
	}

	if err := h.permSvc.UpdatePermissionLevel(name, permID, req.MinLevel); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{"code": "UPDATE_ERROR", "message": err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{"message": "권한이 업데이트되었습니다"}})
}
