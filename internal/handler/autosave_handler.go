package handler

import (
	"net/http"
	"strconv"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/internal/service"
	"github.com/gin-gonic/gin"
)

// AutosaveHandler handles autosave requests
type AutosaveHandler struct {
	service service.AutosaveService
}

// NewAutosaveHandler creates a new AutosaveHandler
func NewAutosaveHandler(service service.AutosaveService) *AutosaveHandler {
	return &AutosaveHandler{service: service}
}

// Save handles POST /api/v2/autosave
// @Summary 게시글 자동 저장
// @Description 작성 중인 게시글을 자동으로 저장합니다
// @Tags autosave
// @Accept json
// @Produce json
// @Param request body domain.AutosaveRequest true "저장할 내용"
// @Success 200 {object} common.APIResponse{data=domain.AutosaveResponse}
// @Failure 401 {object} common.APIResponse
// @Failure 400 {object} common.APIResponse
// @Security BearerAuth
// @Router /autosave [post]
func (h *AutosaveHandler) Save(c *gin.Context) {
	// Check authentication (damoang_jwt cookie)
	if !middleware.IsDamoangAuthenticated(c) {
		c.JSON(http.StatusOK, common.APIResponse{Data: gin.H{"count": 0}})
		return
	}

	memberID := middleware.GetDamoangUserID(c)
	if memberID == "" {
		c.JSON(http.StatusOK, common.APIResponse{Data: gin.H{"count": 0}})
		return
	}

	var req domain.AutosaveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다.", err)
		return
	}

	count, err := h.service.Save(memberID, &req)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "저장에 실패했습니다.", err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{
		Data: domain.AutosaveResponse{Count: int(count)},
	})
}

// List handles GET /api/v2/autosave
// @Summary 자동 저장 목록 조회
// @Description 자동 저장된 게시글 목록을 조회합니다
// @Tags autosave
// @Produce json
// @Success 200 {object} common.APIResponse{data=[]domain.AutosaveListItem}
// @Failure 401 {object} common.APIResponse
// @Security BearerAuth
// @Router /autosave [get]
func (h *AutosaveHandler) List(c *gin.Context) {
	// Check authentication
	if !middleware.IsDamoangAuthenticated(c) {
		c.JSON(http.StatusOK, common.APIResponse{Data: []domain.AutosaveListItem{}})
		return
	}

	memberID := middleware.GetDamoangUserID(c)
	if memberID == "" {
		c.JSON(http.StatusOK, common.APIResponse{Data: []domain.AutosaveListItem{}})
		return
	}

	items, err := h.service.List(memberID)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "목록 조회에 실패했습니다.", err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: items})
}

// Load handles GET /api/v2/autosave/:id
// @Summary 자동 저장 불러오기
// @Description 자동 저장된 게시글을 불러옵니다
// @Tags autosave
// @Produce json
// @Param id path int true "자동저장 ID"
// @Success 200 {object} common.APIResponse{data=domain.AutosaveDetail}
// @Failure 401 {object} common.APIResponse
// @Failure 404 {object} common.APIResponse
// @Security BearerAuth
// @Router /autosave/{id} [get]
func (h *AutosaveHandler) Load(c *gin.Context) {
	// Check authentication
	if !middleware.IsDamoangAuthenticated(c) {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다.", nil)
		return
	}

	memberID := middleware.GetDamoangUserID(c)
	if memberID == "" {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다.", nil)
		return
	}

	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "잘못된 ID입니다.", err)
		return
	}

	detail, err := h.service.Load(id, memberID)
	if err != nil {
		common.ErrorResponse(c, http.StatusNotFound, "자동 저장 데이터를 찾을 수 없습니다.", err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: detail})
}

// Delete handles DELETE /api/v2/autosave/:id
// @Summary 자동 저장 삭제
// @Description 자동 저장된 게시글을 삭제합니다
// @Tags autosave
// @Produce json
// @Param id path int true "자동저장 ID"
// @Success 200 {object} common.APIResponse{data=domain.AutosaveResponse}
// @Failure 401 {object} common.APIResponse
// @Failure 500 {object} common.APIResponse
// @Security BearerAuth
// @Router /autosave/{id} [delete]
func (h *AutosaveHandler) Delete(c *gin.Context) {
	// Check authentication
	if !middleware.IsDamoangAuthenticated(c) {
		c.JSON(http.StatusOK, common.APIResponse{Data: gin.H{"count": 0}})
		return
	}

	memberID := middleware.GetDamoangUserID(c)
	if memberID == "" {
		c.JSON(http.StatusOK, common.APIResponse{Data: gin.H{"count": 0}})
		return
	}

	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "잘못된 ID입니다.", err)
		return
	}

	count, err := h.service.Delete(id, memberID)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "삭제에 실패했습니다.", err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{
		Data: domain.AutosaveResponse{Count: int(count)},
	})
}
