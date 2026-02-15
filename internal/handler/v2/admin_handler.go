package v2

import (
	"net/http"
	"strconv"

	"github.com/damoang/angple-backend/internal/common"
	v2domain "github.com/damoang/angple-backend/internal/domain/v2"
	v2svc "github.com/damoang/angple-backend/internal/service/v2"
	"github.com/gin-gonic/gin"
)

// AdminHandler handles v2 admin API endpoints
type AdminHandler struct {
	adminService *v2svc.AdminService
}

// NewAdminHandler creates a new AdminHandler
func NewAdminHandler(adminService *v2svc.AdminService) *AdminHandler {
	return &AdminHandler{adminService: adminService}
}

// ListBoards handles GET /api/v2/admin/boards
func (h *AdminHandler) ListBoards(c *gin.Context) {
	boards, err := h.adminService.ListAllBoards()
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "게시판 목록 조회 실패", err)
		return
	}
	common.V2Success(c, boards)
}

// CreateBoard handles POST /api/v2/admin/boards
func (h *AdminHandler) CreateBoard(c *gin.Context) {
	var req struct {
		Slug        string  `json:"slug" binding:"required"`
		Name        string  `json:"name" binding:"required"`
		Description *string `json:"description"`
		CategoryID  *uint64 `json:"category_id"`
		Settings    *string `json:"settings"`
		IsActive    *bool   `json:"is_active"`
		OrderNum    *uint   `json:"order_num"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다", err)
		return
	}

	board := &v2domain.V2Board{
		Slug:        req.Slug,
		Name:        req.Name,
		Description: req.Description,
		CategoryID:  req.CategoryID,
		Settings:    req.Settings,
		IsActive:    true,
	}
	if req.IsActive != nil {
		board.IsActive = *req.IsActive
	}
	if req.OrderNum != nil {
		board.OrderNum = *req.OrderNum
	}

	if err := h.adminService.CreateBoard(board); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "게시판 생성 실패", err)
		return
	}
	common.V2Created(c, board)
}

// UpdateBoard handles PUT /api/v2/admin/boards/:id
func (h *AdminHandler) UpdateBoard(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 게시판 ID", err)
		return
	}

	var req struct {
		Slug        *string `json:"slug"`
		Name        *string `json:"name"`
		Description *string `json:"description"`
		CategoryID  *uint64 `json:"category_id"`
		Settings    *string `json:"settings"`
		IsActive    *bool   `json:"is_active"`
		OrderNum    *uint   `json:"order_num"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다", err)
		return
	}

	board := &v2domain.V2Board{ID: id}
	if req.Slug != nil {
		board.Slug = *req.Slug
	}
	if req.Name != nil {
		board.Name = *req.Name
	}
	board.Description = req.Description
	board.CategoryID = req.CategoryID
	board.Settings = req.Settings
	if req.IsActive != nil {
		board.IsActive = *req.IsActive
	}
	if req.OrderNum != nil {
		board.OrderNum = *req.OrderNum
	}

	if err := h.adminService.UpdateBoard(board); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "게시판 수정 실패", err)
		return
	}
	common.V2Success(c, board)
}

// DeleteBoard handles DELETE /api/v2/admin/boards/:id
func (h *AdminHandler) DeleteBoard(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 게시판 ID", err)
		return
	}
	if err := h.adminService.DeleteBoard(id); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "게시판 삭제 실패", err)
		return
	}
	common.V2Success(c, gin.H{"message": "삭제 완료"})
}

// ListMembers handles GET /api/v2/admin/members
func (h *AdminHandler) ListMembers(c *gin.Context) {
	page, perPage := parsePagination(c)
	keyword := c.Query("keyword")

	users, total, err := h.adminService.ListMembers(page, perPage, keyword)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "회원 목록 조회 실패", err)
		return
	}
	common.V2SuccessWithMeta(c, users, common.NewV2Meta(page, perPage, total))
}

// GetMember handles GET /api/v2/admin/members/:id
func (h *AdminHandler) GetMember(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 회원 ID", err)
		return
	}
	user, err := h.adminService.GetMember(id)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusNotFound, "회원을 찾을 수 없습니다", err)
		return
	}
	common.V2Success(c, user)
}

// UpdateMember handles PUT /api/v2/admin/members/:id
func (h *AdminHandler) UpdateMember(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 회원 ID", err)
		return
	}

	user, err := h.adminService.GetMember(id)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusNotFound, "회원을 찾을 수 없습니다", err)
		return
	}

	var req struct {
		Nickname *string `json:"nickname"`
		Level    *uint8  `json:"level"`
		Status   *string `json:"status"`
		Bio      *string `json:"bio"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다", err)
		return
	}

	if req.Nickname != nil {
		user.Nickname = *req.Nickname
	}
	if req.Level != nil {
		user.Level = *req.Level
	}
	if req.Status != nil {
		user.Status = *req.Status
	}
	if req.Bio != nil {
		user.Bio = req.Bio
	}

	if err := h.adminService.UpdateMember(user); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "회원 수정 실패", err)
		return
	}
	common.V2Success(c, user)
}

// BanMember handles POST /api/v2/admin/members/:id/ban
func (h *AdminHandler) BanMember(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 회원 ID", err)
		return
	}

	var req struct {
		Ban bool `json:"ban"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다", err)
		return
	}

	if err := h.adminService.BanMember(id, req.Ban); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "회원 차단/해제 실패", err)
		return
	}

	msg := "차단 완료"
	if !req.Ban {
		msg = "차단 해제 완료"
	}
	common.V2Success(c, gin.H{"message": msg})
}

// GetDashboardStats handles GET /api/v2/admin/dashboard/stats
func (h *AdminHandler) GetDashboardStats(c *gin.Context) {
	stats, err := h.adminService.GetDashboardStats()
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "대시보드 통계 조회 실패", err)
		return
	}
	common.V2Success(c, stats)
}
