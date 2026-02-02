package handler

import (
	"net/http"
	"strconv"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/service"
	"github.com/gin-gonic/gin"
)

// GalleryHandler handles gallery and unified search requests
type GalleryHandler struct {
	service service.GalleryService
}

// NewGalleryHandler creates a new GalleryHandler
func NewGalleryHandler(service service.GalleryService) *GalleryHandler {
	return &GalleryHandler{service: service}
}

// GetGallery handles GET /gallery/:board_id
// @Summary 게시판 갤러리
// @Tags gallery
// @Produce json
// @Param board_id path string true "게시판 ID"
// @Param page query int false "페이지"
// @Param limit query int false "페이지 크기"
// @Success 200 {object} common.APIResponse{data=[]domain.GalleryItem}
// @Router /gallery/{board_id} [get]
func (h *GalleryHandler) GetGallery(c *gin.Context) {
	boardID := c.Param("board_id")
	if boardID == "" {
		common.ErrorResponse(c, http.StatusBadRequest, "게시판 ID가 필요합니다", nil)
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

	items, meta, err := h.service.GetGallery(boardID, page, limit)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "갤러리 조회 실패", err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: items, Meta: meta})
}

// GetGalleryAll handles GET /gallery
// @Summary 전체 갤러리
// @Tags gallery
// @Produce json
// @Param page query int false "페이지"
// @Param limit query int false "페이지 크기"
// @Success 200 {object} common.APIResponse{data=[]domain.GalleryItem}
// @Router /gallery [get]
func (h *GalleryHandler) GetGalleryAll(c *gin.Context) {
	page := 1
	if p, err := strconv.Atoi(c.Query("page")); err == nil && p > 0 {
		page = p
	}
	limit := 20
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 {
		limit = l
	}

	items, meta, err := h.service.GetGalleryAll(page, limit)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "갤러리 조회 실패", err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: items, Meta: meta})
}

// UnifiedSearch handles GET /search
// @Summary 통합 검색
// @Tags search
// @Produce json
// @Param q query string true "검색어"
// @Param page query int false "페이지"
// @Param limit query int false "페이지 크기"
// @Success 200 {object} common.APIResponse{data=[]domain.UnifiedSearchResult}
// @Router /search [get]
func (h *GalleryHandler) UnifiedSearch(c *gin.Context) {
	keyword := c.Query("q")
	if keyword == "" {
		common.ErrorResponse(c, http.StatusBadRequest, "검색어가 필요합니다", nil)
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

	results, meta, err := h.service.UnifiedSearch(keyword, page, limit)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "검색 실패", err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: results, Meta: meta})
}
