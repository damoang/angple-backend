package v2

import (
	"net/http"
	"strconv"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/middleware"
	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
	"github.com/gin-gonic/gin"
)

// ExpHandler handles experience point-related endpoints
type ExpHandler struct {
	expRepo v2repo.ExpRepository
}

// NewExpHandler creates a new ExpHandler
func NewExpHandler(expRepo v2repo.ExpRepository) *ExpHandler {
	return &ExpHandler{expRepo: expRepo}
}

// GetExpSummary handles GET /api/v1/my/exp
func (h *ExpHandler) GetExpSummary(c *gin.Context) {
	mbID := middleware.GetUserID(c) // mb_id from JWT
	if mbID == "" {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "인증이 필요합니다", nil)
		return
	}

	summary, err := h.expRepo.GetSummary(mbID)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "경험치 조회에 실패했습니다", err)
		return
	}

	common.V2Success(c, summary)
}

// GetExpHistory handles GET /api/v1/my/exp/history
func (h *ExpHandler) GetExpHistory(c *gin.Context) {
	mbID := middleware.GetUserID(c)
	if mbID == "" {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "인증이 필요합니다", nil)
		return
	}

	// Parse query params
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	history, total, err := h.expRepo.GetHistory(mbID, page, limit)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "경험치 내역 조회에 실패했습니다", err)
		return
	}

	// Get summary as well
	summary, _ := h.expRepo.GetSummary(mbID)

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
