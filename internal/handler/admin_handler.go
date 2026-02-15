package handler

import (
	"net/http"
	"strconv"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/service"
	"github.com/gin-gonic/gin"
)

// AdminHandler handles admin member management requests
type AdminHandler struct {
	service service.AdminMemberService
}

// NewAdminHandler creates a new AdminHandler
func NewAdminHandler(service service.AdminMemberService) *AdminHandler {
	return &AdminHandler{service: service}
}

// ListMembers handles GET /api/v2/admin/members
func (h *AdminHandler) ListMembers(c *gin.Context) {
	page := 1
	if p, err := strconv.Atoi(c.Query("page")); err == nil && p > 0 {
		page = p
	}
	limit := 20
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 {
		limit = l
	}
	keyword := c.Query("keyword")

	items, meta, err := h.service.ListMembers(page, limit, keyword)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "회원 목록 조회 실패", err)
		return
	}
	c.JSON(http.StatusOK, common.APIResponse{Data: items, Meta: meta})
}

// GetMember handles GET /api/v2/admin/members/:id
func (h *AdminHandler) GetMember(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "잘못된 회원 ID", err)
		return
	}

	detail, err := h.service.GetMember(id)
	if err != nil {
		common.ErrorResponse(c, http.StatusNotFound, "회원을 찾을 수 없습니다", err)
		return
	}
	c.JSON(http.StatusOK, common.APIResponse{Data: detail})
}

// UpdateMember handles PUT /api/v2/admin/members/:id
func (h *AdminHandler) UpdateMember(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "잘못된 회원 ID", err)
		return
	}

	var req domain.AdminMemberUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다", err)
		return
	}

	if err := h.service.UpdateMember(id, &req); err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "회원 수정 실패", err)
		return
	}
	c.JSON(http.StatusOK, common.APIResponse{Data: gin.H{"message": "수정 완료"}})
}

// AdjustPoint handles POST /api/v2/admin/members/:id/point
func (h *AdminHandler) AdjustPoint(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "잘못된 회원 ID", err)
		return
	}

	var req domain.AdminPointAdjustRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다", err)
		return
	}

	if err := h.service.AdjustPoint(id, &req); err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "포인트 조정 실패", err)
		return
	}
	c.JSON(http.StatusOK, common.APIResponse{Data: gin.H{"message": "포인트 조정 완료"}})
}

// RestrictMember handles POST /api/v2/admin/members/:id/restrict
func (h *AdminHandler) RestrictMember(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "잘못된 회원 ID", err)
		return
	}

	var req domain.AdminRestrictRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다", err)
		return
	}

	if err := h.service.RestrictMember(id, &req); err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "이용제한 처리 실패", err)
		return
	}
	c.JSON(http.StatusOK, common.APIResponse{Data: gin.H{"message": "이용제한 처리 완료"}})
}
