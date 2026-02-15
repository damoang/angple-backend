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

// DisciplineHandler handles discipline/ban HTTP requests
type DisciplineHandler struct {
	service service.DisciplineService
}

// NewDisciplineHandler creates a new DisciplineHandler
func NewDisciplineHandler(service service.DisciplineService) *DisciplineHandler {
	return &DisciplineHandler{service: service}
}

// MyDisciplines handles GET /members/me/disciplines
// @Summary 내 이용제한 내역
// @Tags discipline
// @Produce json
// @Param page query int false "페이지"
// @Param limit query int false "페이지 크기"
// @Success 200 {object} common.APIResponse{data=[]domain.DisciplineResponse}
// @Router /members/me/disciplines [get]
func (h *DisciplineHandler) MyDisciplines(c *gin.Context) {
	userID := middleware.GetDamoangUserID(c)
	if userID == "" {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	page := 1
	if p, err := strconv.Atoi(c.Query("page")); err == nil && p > 0 {
		page = p
	}
	limit := 20
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 {
		limit = l
	}

	disciplines, meta, err := h.service.GetMyDisciplines(userID, page, limit)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "이용제한 내역 조회 실패", err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: disciplines, Meta: meta})
}

// GetDiscipline handles GET /disciplines/:id
// @Summary 이용제한 상세 열람
// @Tags discipline
// @Produce json
// @Param id path int true "이용제한 ID"
// @Success 200 {object} common.APIResponse{data=domain.DisciplineResponse}
// @Router /disciplines/{id} [get]
func (h *DisciplineHandler) GetDiscipline(c *gin.Context) {
	userID := middleware.GetDamoangUserID(c)
	if userID == "" {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "잘못된 ID입니다", err)
		return
	}

	discipline, err := h.service.GetDiscipline(id, userID)
	if err != nil {
		common.ErrorResponse(c, http.StatusNotFound, err.Error(), err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: discipline})
}

// SubmitAppeal handles POST /disciplines/:id/appeal
// @Summary 소명 글 작성
// @Tags discipline
// @Accept json
// @Produce json
// @Param id path int true "이용제한 ID"
// @Param request body domain.AppealRequest true "소명 내용"
// @Success 200 {object} common.APIResponse
// @Router /disciplines/{id}/appeal [post]
func (h *DisciplineHandler) SubmitAppeal(c *gin.Context) {
	userID := middleware.GetDamoangUserID(c)
	if userID == "" {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "잘못된 ID입니다", err)
		return
	}

	var req domain.AppealRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다", err)
		return
	}

	nickname := middleware.GetDamoangUserName(c)
	appealID, err := h.service.SubmitAppeal(id, userID, nickname, req.Content, c.ClientIP())
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, err.Error(), err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{
		Data: gin.H{"appeal_id": appealID, "message": "소명이 접수되었습니다"},
	})
}

// ListBoard handles GET /disciplines/board
// @Summary 이용제한 게시판
// @Tags discipline
// @Produce json
// @Param page query int false "페이지"
// @Param limit query int false "페이지 크기"
// @Success 200 {object} common.APIResponse{data=[]domain.DisciplineResponse}
// @Router /disciplines/board [get]
func (h *DisciplineHandler) ListBoard(c *gin.Context) {
	page := 1
	if p, err := strconv.Atoi(c.Query("page")); err == nil && p > 0 {
		page = p
	}
	limit := 20
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 {
		limit = l
	}

	disciplines, meta, err := h.service.ListBoard(page, limit)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "이용제한 게시판 조회 실패", err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: disciplines, Meta: meta})
}
