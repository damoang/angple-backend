
// 나눔 플러그인 공개 + 인증 핸들러
package handlers

import (
	"math"
	"net/http"
	"strconv"

	"github.com/damoang/angple-backend/plugins/giving/service"

	"github.com/gin-gonic/gin"
)

// PublicHandler 나눔 공개 API 핸들러
type PublicHandler struct {
	svc service.GivingService
}

// NewPublicHandler 핸들러 생성자
func NewPublicHandler(svc service.GivingService) *PublicHandler {
	return &PublicHandler{svc: svc}
}

// ListGivings 나눔 목록
// GET /api/plugins/giving/list?tab=active&page=1&limit=20&sort=urgent
func (h *PublicHandler) ListGivings(c *gin.Context) {
	tab := c.DefaultQuery("tab", "active")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	sort := c.DefaultQuery("sort", "urgent")

	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 40 {
		limit = 40
	}

	items, total, err := h.svc.ListGivings(tab, sort, page, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to fetch giving data",
		})
		return
	}

	totalPages := int(math.Ceil(float64(total) / float64(limit)))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    items,
		"pagination": gin.H{
			"page":       page,
			"total":      total,
			"totalPages": totalPages,
			"limit":      limit,
		},
	})
}

// GetGivingDetail 나눔 상세 (통계 + 당첨자)
// GET /api/plugins/giving/:id
func (h *PublicHandler) GetGivingDetail(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid post ID",
		})
		return
	}

	// optional auth: 사용자 ID가 있을 수도 없을 수도
	mbID, _ := c.Get("userID")
	mbIDStr := ""
	if mbID != nil {
		if s, ok := mbID.(string); ok {
			mbIDStr = s
		}
	}

	detail, err := h.svc.GetDetail(id, mbIDStr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to fetch giving detail",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    detail,
	})
}

// CreateBid 나눔 응모
// POST /api/plugins/giving/:id/bid
func (h *PublicHandler) CreateBid(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid post ID",
		})
		return
	}

	mbID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "로그인이 필요합니다.",
		})
		return
	}

	nickname, _ := c.Get("nickname")
	mbIDStr := mbID.(string)
	nickStr := ""
	if nickname != nil {
		if s, ok := nickname.(string); ok {
			nickStr = s
		}
	}

	var req struct {
		Numbers string `json:"numbers" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "번호를 입력해주세요.",
		})
		return
	}

	result, err := h.svc.CreateBid(id, mbIDStr, nickStr, req.Numbers)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// GetMyBids 내 응모 현황
// GET /api/plugins/giving/:id/bid
func (h *PublicHandler) GetMyBids(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid post ID",
		})
		return
	}

	mbID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "로그인이 필요합니다.",
		})
		return
	}

	bids, err := h.svc.GetMyBids(id, mbID.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to fetch bids",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    bids,
	})
}

// GetVisualization 종료된 나눔 번호 분포
// GET /api/plugins/giving/:id/visualization
func (h *PublicHandler) GetVisualization(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid post ID",
		})
		return
	}

	result, err := h.svc.GetVisualization(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// GetLiveStatus 진행중 나눔 실시간 현황
// GET /api/plugins/giving/:id/live-status
func (h *PublicHandler) GetLiveStatus(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid post ID",
		})
		return
	}

	result, err := h.svc.GetLiveStatus(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to fetch live status",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}
