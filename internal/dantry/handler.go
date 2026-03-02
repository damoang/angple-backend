package dantry

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/gin-gonic/gin"
)

// Handler handles HTTP requests for Dantry error viewing
type Handler struct {
	repo *Repository
}

// NewHandler creates a new Dantry Handler
func NewHandler(repo *Repository) *Handler {
	return &Handler{repo: repo}
}

// requireAdmin checks if the current user has admin level
func requireAdmin(c *gin.Context) bool {
	level := middleware.GetUserLevel(c)
	if level < 10 {
		common.V2ErrorResponse(c, http.StatusForbidden, "관리자 권한이 필요합니다", nil)
		return false
	}
	return true
}

// ListErrors handles GET /api/v1/dantry/errors
func (h *Handler) ListErrors(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	dateFrom := c.DefaultQuery("date_from", time.Now().AddDate(0, 0, -7).Format("2006-01-02"))
	dateTo := c.DefaultQuery("date_to", time.Now().Format("2006-01-02"))
	errorType := c.Query("type")
	search := c.Query("search")
	excludeScriptError := c.DefaultQuery("exclude_script_error", "true") == "true"

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if limit < 1 || limit > 100 {
		limit = 20
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	errors, total, err := h.repo.ListErrors(ctx, dateFrom, dateTo, errorType, search, excludeScriptError, page, limit)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "에러 목록 조회 실패", err)
		return
	}

	common.V2SuccessWithMeta(c, errors, common.NewV2Meta(page, limit, total))
}

// ListGrouped handles GET /api/v1/dantry/errors/grouped
func (h *Handler) ListGrouped(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	dateFrom := c.DefaultQuery("date_from", time.Now().AddDate(0, 0, -7).Format("2006-01-02"))
	dateTo := c.DefaultQuery("date_to", time.Now().Format("2006-01-02"))
	errorType := c.Query("type")
	search := c.Query("search")
	excludeScriptError := c.DefaultQuery("exclude_script_error", "true") == "true"

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if limit < 1 || limit > 100 {
		limit = 20
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	groups, total, err := h.repo.ListGrouped(ctx, dateFrom, dateTo, errorType, search, excludeScriptError, page, limit)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "그룹 에러 조회 실패", err)
		return
	}

	common.V2SuccessWithMeta(c, groups, common.NewV2Meta(page, limit, total))
}

// GetByID handles GET /api/v1/dantry/errors/:id
func (h *Handler) GetByID(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	id := c.Param("id")
	if id == "" {
		common.V2ErrorResponse(c, http.StatusBadRequest, "ID가 필요합니다", nil)
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	dantryError, err := h.repo.GetByID(ctx, id)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusNotFound, "에러를 찾을 수 없습니다", err)
		return
	}

	common.V2Success(c, dantryError)
}

// GetStats handles GET /api/v1/dantry/stats
func (h *Handler) GetStats(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	stats, err := h.repo.GetStats(ctx)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "통계 조회 실패", err)
		return
	}

	common.V2Success(c, stats)
}

// GetTimeseries handles GET /api/v1/dantry/timeseries
func (h *Handler) GetTimeseries(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	dateFrom := c.DefaultQuery("date_from", time.Now().AddDate(0, 0, -7).Format("2006-01-02"))
	dateTo := c.DefaultQuery("date_to", time.Now().Format("2006-01-02"))
	excludeScriptError := c.DefaultQuery("exclude_script_error", "true") == "true"

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	buckets, err := h.repo.GetTimeseries(ctx, dateFrom, dateTo, excludeScriptError)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "타임시리즈 조회 실패", err)
		return
	}

	common.V2Success(c, buckets)
}
