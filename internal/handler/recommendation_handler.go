package handler

import (
	"net/http"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/internal/service"
	"github.com/gin-gonic/gin"
)

// RecommendationHandler handles AI recommendation APIs
type RecommendationHandler struct {
	recSvc *service.RecommendationService
}

// NewRecommendationHandler creates a new RecommendationHandler
func NewRecommendationHandler(recSvc *service.RecommendationService) *RecommendationHandler {
	return &RecommendationHandler{recSvc: recSvc}
}

// GetPersonalizedFeed godoc
// @Summary 개인화 피드 조회
// @Tags recommendation
// @Param limit query int false "결과 수" default(20)
// @Success 200 {object} common.V2Response
// @Router /api/v2/recommendations/feed [get]
func (h *RecommendationHandler) GetPersonalizedFeed(c *gin.Context) {
	userID := middleware.GetDamoangUserID(c)
	if userID == "" {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	limit := parseIntQuery(c, "limit", 20)
	feed, err := h.recSvc.GetPersonalizedFeed(c.Request.Context(), userID, limit)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "피드 조회 실패", err)
		return
	}
	common.V2Success(c, feed)
}

// TrackActivity godoc
// @Summary 사용자 행동 기록
// @Tags recommendation
// @Accept json
// @Success 200 {object} common.V2Response
// @Router /api/v2/recommendations/track [post]
func (h *RecommendationHandler) TrackActivity(c *gin.Context) {
	userID := middleware.GetDamoangUserID(c)
	if userID == "" {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	var req struct {
		ActionType string `json:"action_type" binding:"required"`
		TargetType string `json:"target_type" binding:"required"`
		TargetID   string `json:"target_id" binding:"required"`
		BoardID    string `json:"board_id"`
		Metadata   string `json:"metadata"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다", err)
		return
	}

	if err := h.recSvc.TrackActivity(c.Request.Context(), userID, req.ActionType, req.TargetType, req.TargetID, req.BoardID, req.Metadata); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "행동 기록 실패", err)
		return
	}
	common.V2Success(c, gin.H{"message": "기록되었습니다"})
}

// GetTrendingTopics godoc
// @Summary 트렌딩 토픽 조회
// @Tags recommendation
// @Param period query string false "기간 (24h, 7d, 30d)" default(24h)
// @Param limit query int false "결과 수" default(20)
// @Success 200 {object} common.V2Response
// @Router /api/v2/recommendations/trending [get]
func (h *RecommendationHandler) GetTrendingTopics(c *gin.Context) {
	period := c.DefaultQuery("period", "24h")
	limit := parseIntQuery(c, "limit", 20)

	topics, err := h.recSvc.GetTrendingTopics(c.Request.Context(), period, limit)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "트렌딩 조회 실패", err)
		return
	}
	common.V2Success(c, topics)
}

// GetUserInterests godoc
// @Summary 사용자 관심 토픽 조회
// @Tags recommendation
// @Param limit query int false "결과 수" default(10)
// @Success 200 {object} common.V2Response
// @Router /api/v2/recommendations/interests [get]
func (h *RecommendationHandler) GetUserInterests(c *gin.Context) {
	userID := middleware.GetDamoangUserID(c)
	if userID == "" {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	limit := parseIntQuery(c, "limit", 10)
	interests, err := h.recSvc.GetUserInterests(c.Request.Context(), userID, limit)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "관심 토픽 조회 실패", err)
		return
	}
	common.V2Success(c, interests)
}

// ExtractTopics godoc
// @Summary 게시글 토픽 추출 (관리자)
// @Tags recommendation
// @Accept json
// @Success 200 {object} common.V2Response
// @Router /api/v2/admin/recommendations/extract [post]
func (h *RecommendationHandler) ExtractTopics(c *gin.Context) {
	var req struct {
		BoardID string `json:"board_id" binding:"required"`
		PostID  string `json:"post_id" binding:"required"`
		Title   string `json:"title" binding:"required"`
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다", err)
		return
	}

	if err := h.recSvc.ExtractAndSaveTopics(c.Request.Context(), req.BoardID, req.PostID, req.Title, req.Content); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "토픽 추출 실패", err)
		return
	}
	common.V2Success(c, gin.H{"message": "토픽이 추출되었습니다"})
}

// RefreshTrending godoc
// @Summary 트렌딩 갱신 (관리자)
// @Tags recommendation
// @Success 200 {object} common.V2Response
// @Router /api/v2/admin/recommendations/refresh-trending [post]
func (h *RecommendationHandler) RefreshTrending(c *gin.Context) {
	if err := h.recSvc.RefreshTrending(c.Request.Context()); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	common.V2Success(c, gin.H{"message": "트렌딩이 갱신되었습니다"})
}
