package v2

import (
	"net/http"
	"strconv"

	"github.com/damoang/angple-backend/internal/middleware"
	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
	"github.com/gin-gonic/gin"
)

// MyHandler handles /api/v1/my/* endpoints
type MyHandler struct {
	pointRepo v2repo.MyPointRepository
	expRepo   v2repo.MyExpRepository
}

// NewMyHandler creates a new MyHandler
func NewMyHandler(pointRepo v2repo.MyPointRepository, expRepo v2repo.MyExpRepository) *MyHandler {
	return &MyHandler{
		pointRepo: pointRepo,
		expRepo:   expRepo,
	}
}

// GetMyPoint handles GET /api/v1/my/point — 포인트 요약
func (h *MyHandler) GetMyPoint(c *gin.Context) {
	mbID := middleware.GetUserID(c)
	if mbID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "로그인이 필요합니다"})
		return
	}

	summary, err := h.pointRepo.GetSummary(mbID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "포인트 조회 실패"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": summary})
}

// GetMyPointHistory handles GET /api/v1/my/point/history — 포인트 내역
func (h *MyHandler) GetMyPointHistory(c *gin.Context) {
	mbID := middleware.GetUserID(c)
	if mbID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "로그인이 필요합니다"})
		return
	}

	page, limit := parsePageLimit(c)

	summary, err := h.pointRepo.GetSummary(mbID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "포인트 조회 실패"})
		return
	}

	items, total, err := h.pointRepo.GetHistory(mbID, page, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "포인트 내역 조회 실패"})
		return
	}

	totalPages := total / int64(limit)
	if total%int64(limit) > 0 {
		totalPages++
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"summary":     summary,
			"items":       items,
			"total":       total,
			"page":        page,
			"limit":       limit,
			"total_pages": totalPages,
		},
	})
}

// GetMyExp handles GET /api/v1/my/exp — 경험치 요약
func (h *MyHandler) GetMyExp(c *gin.Context) {
	mbID := middleware.GetUserID(c)
	if mbID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "로그인이 필요합니다"})
		return
	}

	summary, err := h.expRepo.GetSummary(mbID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "경험치 조회 실패"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": summary})
}

// GetMyExpHistory handles GET /api/v1/my/exp/history — 경험치 내역
func (h *MyHandler) GetMyExpHistory(c *gin.Context) {
	mbID := middleware.GetUserID(c)
	if mbID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "로그인이 필요합니다"})
		return
	}

	page, limit := parsePageLimit(c)

	summary, err := h.expRepo.GetSummary(mbID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "경험치 조회 실패"})
		return
	}

	items, total, err := h.expRepo.GetHistory(mbID, page, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "경험치 내역 조회 실패"})
		return
	}

	totalPages := total / int64(limit)
	if total%int64(limit) > 0 {
		totalPages++
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"summary":     summary,
			"items":       items,
			"total":       total,
			"page":        page,
			"limit":       limit,
			"total_pages": totalPages,
		},
	})
}

func parsePageLimit(c *gin.Context) (int, int) {
	page := 1
	if p, err := strconv.Atoi(c.Query("page")); err == nil && p > 0 {
		page = p
	}
	limit := 20
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 && l <= 100 {
		limit = l
	}
	return page, limit
}
