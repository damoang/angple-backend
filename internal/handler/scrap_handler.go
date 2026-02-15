package handler

import (
	"net/http"
	"strconv"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/internal/service"
	"github.com/gin-gonic/gin"
)

// ScrapHandler handles scrap HTTP requests
type ScrapHandler struct {
	service service.ScrapService
}

// NewScrapHandler creates a new ScrapHandler
func NewScrapHandler(service service.ScrapService) *ScrapHandler {
	return &ScrapHandler{service: service}
}

// AddScrap handles POST /boards/:board_id/posts/:id/scrap
// @Summary 게시글 스크랩
// @Tags scrap
// @Produce json
// @Param board_id path string true "게시판 ID"
// @Param id path int true "게시글 ID"
// @Success 200 {object} common.APIResponse{data=domain.ScrapResponse}
// @Router /boards/{board_id}/posts/{id}/scrap [post]
func (h *ScrapHandler) AddScrap(c *gin.Context) {
	userID := middleware.GetDamoangUserID(c)
	if userID == "" {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	boardID := c.Param("board_id")
	wrID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "잘못된 게시글 ID입니다", err)
		return
	}

	result, err := h.service.AddScrap(userID, boardID, wrID)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, err.Error(), err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: result})
}

// RemoveScrap handles DELETE /boards/:board_id/posts/:id/scrap
// @Summary 스크랩 취소
// @Tags scrap
// @Produce json
// @Param board_id path string true "게시판 ID"
// @Param id path int true "게시글 ID"
// @Success 204
// @Router /boards/{board_id}/posts/{id}/scrap [delete]
func (h *ScrapHandler) RemoveScrap(c *gin.Context) {
	userID := middleware.GetDamoangUserID(c)
	if userID == "" {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	boardID := c.Param("board_id")
	wrID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "잘못된 게시글 ID입니다", err)
		return
	}

	if err := h.service.RemoveScrap(userID, boardID, wrID); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, err.Error(), err)
		return
	}

	c.Status(http.StatusNoContent)
}

// ListScraps handles GET /members/me/scraps
// @Summary 내 스크랩 목록
// @Tags scrap
// @Produce json
// @Param page query int false "페이지 (기본 1)"
// @Param limit query int false "페이지 크기 (기본 20)"
// @Success 200 {object} common.APIResponse{data=[]domain.ScrapResponse}
// @Router /members/me/scraps [get]
func (h *ScrapHandler) ListScraps(c *gin.Context) {
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

	scraps, meta, err := h.service.ListScraps(userID, page, limit)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "스크랩 목록 조회 실패", err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: scraps, Meta: meta})
}
