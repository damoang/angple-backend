package v2

import (
	"net/http"
	"strconv"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/middleware"
	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
	"github.com/gin-gonic/gin"
)

// PointHandler handles point-related endpoints
type PointHandler struct {
	gnuPointRepo v2repo.GnuboardPointRepository
}

// NewPointHandler creates a new PointHandler
func NewPointHandler(gnuPointRepo v2repo.GnuboardPointRepository) *PointHandler {
	return &PointHandler{gnuPointRepo: gnuPointRepo}
}

// GetPointSummary handles GET /api/v1/my/point
func (h *PointHandler) GetPointSummary(c *gin.Context) {
	mbID := middleware.GetUserID(c)
	if mbID == "" {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "인증이 필요합니다", nil)
		return
	}

	summary, err := h.gnuPointRepo.GetSummary(mbID)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "포인트 조회에 실패했습니다", err)
		return
	}

	common.V2Success(c, summary)
}

// GetPointHistory handles GET /api/v1/my/point/history
func (h *PointHandler) GetPointHistory(c *gin.Context) {
	mbID := middleware.GetUserID(c)
	if mbID == "" {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "인증이 필요합니다", nil)
		return
	}

	// Parse query params
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	filter := c.DefaultQuery("filter", "all") // all, earned, used

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	history, total, err := h.gnuPointRepo.GetHistory(mbID, filter, page, limit)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "포인트 내역 조회에 실패했습니다", err)
		return
	}

	// Get summary as well
	summary, _ := h.gnuPointRepo.GetSummary(mbID)

	totalPages := (common.SafeInt64ToInt(total) + limit - 1) / limit

	common.V2Success(c, gin.H{
		"summary": summary,
		"items":   history,
		"pagination": gin.H{
			"page":        page,
			"limit":       limit,
			"total":       total,
			"total_pages": totalPages,
		},
	})
}
