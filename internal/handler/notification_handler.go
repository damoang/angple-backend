package handler

import (
	"net/http"
	"strconv"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/internal/service"
	"github.com/gin-gonic/gin"
)

// NotificationHandler handles notification requests
type NotificationHandler struct {
	service *service.NotificationService
}

// NewNotificationHandler creates a new NotificationHandler
func NewNotificationHandler(service *service.NotificationService) *NotificationHandler {
	return &NotificationHandler{service: service}
}

// GetUnreadCount handles GET /api/v2/notifications/unread-count
func (h *NotificationHandler) GetUnreadCount(c *gin.Context) {
	if !middleware.IsDamoangAuthenticated(c) {
		common.ErrorResponse(c, http.StatusForbidden, "회원만 이용할 수 있습니다", nil)
		return
	}

	memberID := middleware.GetDamoangUserID(c)
	summary, err := h.service.GetUnreadCount(memberID)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "알림 수 조회 중 오류가 발생했습니다", err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: summary})
}

// GetList handles GET /api/v2/notifications
func (h *NotificationHandler) GetList(c *gin.Context) {
	if !middleware.IsDamoangAuthenticated(c) {
		common.ErrorResponse(c, http.StatusForbidden, "회원만 이용할 수 있습니다", nil)
		return
	}

	memberID := middleware.GetDamoangUserID(c)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	result, err := h.service.GetList(memberID, page, limit)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "알림 목록 조회 중 오류가 발생했습니다", err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: result})
}

// MarkAsRead handles POST /api/v2/notifications/:id/read
func (h *NotificationHandler) MarkAsRead(c *gin.Context) {
	if !middleware.IsDamoangAuthenticated(c) {
		common.ErrorResponse(c, http.StatusForbidden, "회원만 이용할 수 있습니다", nil)
		return
	}

	memberID := middleware.GetDamoangUserID(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "잘못된 알림 ID입니다", nil)
		return
	}

	if err := h.service.MarkAsRead(memberID, id); err != nil {
		if err.Error() == "notification not found" {
			common.ErrorResponse(c, http.StatusNotFound, "알림을 찾을 수 없습니다", nil)
			return
		}
		if err.Error() == "forbidden" {
			common.ErrorResponse(c, http.StatusForbidden, "권한이 없습니다", nil)
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "알림 읽음 처리 중 오류가 발생했습니다", err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: gin.H{"success": true}})
}

// MarkAllAsRead handles POST /api/v2/notifications/read-all
func (h *NotificationHandler) MarkAllAsRead(c *gin.Context) {
	if !middleware.IsDamoangAuthenticated(c) {
		common.ErrorResponse(c, http.StatusForbidden, "회원만 이용할 수 있습니다", nil)
		return
	}

	memberID := middleware.GetDamoangUserID(c)
	if err := h.service.MarkAllAsRead(memberID); err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "전체 읽음 처리 중 오류가 발생했습니다", err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: gin.H{"success": true}})
}

// Delete handles DELETE /api/v2/notifications/:id
func (h *NotificationHandler) Delete(c *gin.Context) {
	if !middleware.IsDamoangAuthenticated(c) {
		common.ErrorResponse(c, http.StatusForbidden, "회원만 이용할 수 있습니다", nil)
		return
	}

	memberID := middleware.GetDamoangUserID(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "잘못된 알림 ID입니다", nil)
		return
	}

	if err := h.service.Delete(memberID, id); err != nil {
		if err.Error() == "notification not found" {
			common.ErrorResponse(c, http.StatusNotFound, "알림을 찾을 수 없습니다", nil)
			return
		}
		if err.Error() == "forbidden" {
			common.ErrorResponse(c, http.StatusForbidden, "권한이 없습니다", nil)
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "알림 삭제 중 오류가 발생했습니다", err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: gin.H{"success": true}})
}
