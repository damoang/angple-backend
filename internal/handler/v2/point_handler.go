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
	pointRepo v2repo.PointRepository
}

// NewPointHandler creates a new PointHandler
func NewPointHandler(pointRepo v2repo.PointRepository) *PointHandler {
	return &PointHandler{pointRepo: pointRepo}
}

// getV2UserID extracts v2_user_id from context (set by middleware)
func getV2UserID(c *gin.Context) (uint64, bool) {
	v2UserIDStr := middleware.GetUserID(c) // This returns userID as string
	if v2UserIDStr == "" {
		return 0, false
	}
	v2UserID, err := strconv.ParseUint(v2UserIDStr, 10, 64)
	if err != nil {
		return 0, false
	}
	return v2UserID, true
}

// GetPointSummary handles GET /api/v1/my/point
func (h *PointHandler) GetPointSummary(c *gin.Context) {
	v2UserID, ok := getV2UserID(c)
	if !ok {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "인증이 필요합니다", nil)
		return
	}

	summary, err := h.pointRepo.GetSummary(v2UserID)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "포인트 조회에 실패했습니다", err)
		return
	}

	common.V2Success(c, summary)
}

// GetPointHistory handles GET /api/v1/my/point/history
func (h *PointHandler) GetPointHistory(c *gin.Context) {
	v2UserID, ok := getV2UserID(c)
	if !ok {
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

	history, total, err := h.pointRepo.GetHistory(v2UserID, filter, page, limit)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "포인트 내역 조회에 실패했습니다", err)
		return
	}

	// Get summary as well
	summary, _ := h.pointRepo.GetSummary(v2UserID)

	totalPages := (int(total) + limit - 1) / limit

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
