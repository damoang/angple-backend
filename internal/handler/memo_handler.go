package handler

import (
	"net/http"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/internal/service"
	"github.com/gin-gonic/gin"
)

// MemoHandler handles member memo requests
type MemoHandler struct {
	service *service.MemoService
}

// NewMemoHandler creates a new MemoHandler
func NewMemoHandler(service *service.MemoService) *MemoHandler {
	return &MemoHandler{service: service}
}

// GetMemo handles GET /api/v2/members/:id/memo
// @Summary 회원 메모 조회
// @Description 특정 회원에 대한 메모를 조회합니다
// @Tags members
// @Produce json
// @Param id path string true "대상 회원 ID"
// @Param token_only query bool false "토큰만 조회"
// @Success 200 {object} common.APIResponse{data=domain.MemoResponse}
// @Failure 401 {object} common.APIResponse
// @Security BearerAuth
// @Router /members/{id}/memo [get]
func (h *MemoHandler) GetMemo(c *gin.Context) {
	// Check authentication
	if !middleware.IsDamoangAuthenticated(c) {
		common.ErrorResponse(c, http.StatusForbidden, "회원만 이용할 수 있습니다", nil)
		return
	}

	memberID := middleware.GetDamoangUserID(c)
	targetID := c.Param("id")
	tokenOnly := c.Query("token_only") == "true"

	// Generate CSRF token
	token := generateToken()

	if tokenOnly {
		c.JSON(http.StatusOK, common.APIResponse{
			Data: domain.MemoResponse{
				Token: token,
			},
		})
		return
	}

	// Don't show memo for self
	if memberID == targetID {
		c.JSON(http.StatusOK, common.APIResponse{
			Data: domain.MemoResponse{
				TargetID: targetID,
				Content:  "",
				Color:    "yellow",
				Token:    token,
			},
		})
		return
	}

	// Get memo from DB
	memo, err := h.service.GetMemo(memberID, targetID)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "메모 조회 중 오류가 발생했습니다", err)
		return
	}

	response := domain.MemoResponse{
		TargetID: targetID,
		Color:    "yellow",
		Token:    token,
	}

	if memo != nil {
		response.Content = memo.Memo
		response.MemoDetail = memo.MemoDetail
		response.Color = memo.Color
		if memo.UpdatedAt != nil {
			response.UpdatedAt = memo.UpdatedAt.Format(time.RFC3339)
		}
	}

	c.JSON(http.StatusOK, common.APIResponse{
		Data: response,
	})
}

// CreateMemo handles POST /api/v2/members/:id/memo
// @Summary 회원 메모 생성
// @Description 특정 회원에 대한 메모를 생성합니다
// @Tags members
// @Accept json
// @Produce json
// @Param id path string true "대상 회원 ID"
// @Param request body domain.MemoRequest true "메모 내용"
// @Success 200 {object} common.APIResponse{data=domain.MemoResponse}
// @Failure 401 {object} common.APIResponse
// @Security BearerAuth
// @Router /members/{id}/memo [post]
func (h *MemoHandler) CreateMemo(c *gin.Context) {
	// Check authentication
	if !middleware.IsDamoangAuthenticated(c) {
		common.ErrorResponse(c, http.StatusForbidden, "회원만 이용할 수 있습니다", nil)
		return
	}

	memberID := middleware.GetDamoangUserID(c)
	targetID := c.Param("id")

	// Don't allow memo for self
	if memberID == targetID {
		common.ErrorResponse(c, http.StatusBadRequest, "본인에게 메모를 작성할 수 없습니다", nil)
		return
	}

	var req domain.MemoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다.", err)
		return
	}

	// Override target_id from path
	req.TargetID = targetID

	// Create or update memo
	memo, err := h.service.CreateOrUpdateMemo(memberID, targetID, &req)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "메모 저장 중 오류가 발생했습니다", err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{
		Data: domain.MemoResponse{
			TargetID:   memo.TargetMemberID,
			Content:    memo.Memo,
			MemoDetail: memo.MemoDetail,
			Color:      memo.Color,
			CreatedAt:  memo.CreatedAt.Format(time.RFC3339),
		},
	})
}

// UpdateMemo handles PUT /api/v2/members/:id/memo
// @Summary 회원 메모 수정
// @Description 특정 회원에 대한 메모를 수정합니다
// @Tags members
// @Accept json
// @Produce json
// @Param id path string true "대상 회원 ID"
// @Param request body domain.MemoRequest true "메모 내용"
// @Success 200 {object} common.APIResponse{data=domain.MemoResponse}
// @Failure 401 {object} common.APIResponse
// @Security BearerAuth
// @Router /members/{id}/memo [put]
func (h *MemoHandler) UpdateMemo(c *gin.Context) {
	// Check authentication
	if !middleware.IsDamoangAuthenticated(c) {
		common.ErrorResponse(c, http.StatusForbidden, "회원만 이용할 수 있습니다", nil)
		return
	}

	memberID := middleware.GetDamoangUserID(c)
	targetID := c.Param("id")

	var req domain.MemoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다.", err)
		return
	}

	req.TargetID = targetID

	// Create or update memo (same as create - upsert)
	memo, err := h.service.CreateOrUpdateMemo(memberID, targetID, &req)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "메모 수정 중 오류가 발생했습니다", err)
		return
	}

	updatedAt := ""
	if memo.UpdatedAt != nil {
		updatedAt = memo.UpdatedAt.Format(time.RFC3339)
	}

	c.JSON(http.StatusOK, common.APIResponse{
		Data: domain.MemoResponse{
			TargetID:   memo.TargetMemberID,
			Content:    memo.Memo,
			MemoDetail: memo.MemoDetail,
			Color:      memo.Color,
			UpdatedAt:  updatedAt,
		},
	})
}

// DeleteMemo handles DELETE /api/v2/members/:id/memo
// @Summary 회원 메모 삭제
// @Description 특정 회원에 대한 메모를 삭제합니다
// @Tags members
// @Produce json
// @Param id path string true "대상 회원 ID"
// @Success 200 {object} common.APIResponse
// @Failure 401 {object} common.APIResponse
// @Security BearerAuth
// @Router /members/{id}/memo [delete]
func (h *MemoHandler) DeleteMemo(c *gin.Context) {
	// Check authentication
	if !middleware.IsDamoangAuthenticated(c) {
		common.ErrorResponse(c, http.StatusForbidden, "회원만 이용할 수 있습니다", nil)
		return
	}

	memberID := middleware.GetDamoangUserID(c)
	targetID := c.Param("id")

	if err := h.service.DeleteMemo(memberID, targetID); err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "메모 삭제 중 오류가 발생했습니다", err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{
		Data: gin.H{"success": true},
	})
}
