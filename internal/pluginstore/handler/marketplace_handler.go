package handler

import (
	"net/http"
	"strconv"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/internal/pluginstore/domain"
	"github.com/damoang/angple-backend/internal/pluginstore/service"
	"github.com/gin-gonic/gin"
)

// getUserIDUint64 extracts user ID and converts to uint64
func getUserIDUint64(c *gin.Context) uint64 {
	idStr := middleware.GetUserID(c)
	if idStr == "" {
		return 0
	}
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		return 0
	}
	return id
}

// MarketplaceHandler 마켓플레이스 HTTP 핸들러
type MarketplaceHandler struct {
	svc *service.MarketplaceService
}

// NewMarketplaceHandler 생성자
func NewMarketplaceHandler(svc *service.MarketplaceService) *MarketplaceHandler {
	return &MarketplaceHandler{svc: svc}
}

// === Public API ===

// Browse godoc
// @Summary 마켓플레이스 탐색
// @Tags marketplace
// @Param page query int false "페이지" default(1)
// @Param per_page query int false "페이지당 항목" default(20)
// @Param category query string false "카테고리 필터"
// @Param keyword query string false "검색 키워드"
// @Success 200 {object} common.V2Response
// @Router /api/v2/marketplace [get]
func (h *MarketplaceHandler) Browse(c *gin.Context) {
	page, perPage := parsePage(c)
	category := c.Query("category")
	keyword := c.Query("keyword")

	items, total, err := h.svc.Browse(page, perPage, category, keyword)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "마켓플레이스 조회 실패", err)
		return
	}
	common.V2SuccessWithMeta(c, items, common.NewV2Meta(page, perPage, total))
}

// GetPlugin godoc
// @Summary 플러그인 상세
// @Tags marketplace
// @Param name path string true "플러그인 이름"
// @Success 200 {object} common.V2Response
// @Router /api/v2/marketplace/{name} [get]
func (h *MarketplaceHandler) GetPlugin(c *gin.Context) {
	name := c.Param("name")
	sub, avgRating, reviewCount, err := h.svc.GetPlugin(name)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusNotFound, "플러그인을 찾을 수 없습니다", err)
		return
	}
	common.V2Success(c, gin.H{
		"plugin":       sub,
		"avg_rating":   avgRating,
		"review_count": reviewCount,
	})
}

// GetReviews godoc
// @Summary 플러그인 리뷰 목록
// @Tags marketplace
// @Param name path string true "플러그인 이름"
// @Success 200 {object} common.V2Response
// @Router /api/v2/marketplace/{name}/reviews [get]
func (h *MarketplaceHandler) GetReviews(c *gin.Context) {
	name := c.Param("name")
	page, perPage := parsePage(c)
	reviews, total, err := h.svc.GetReviews(name, page, perPage)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "리뷰 조회 실패", err)
		return
	}
	common.V2SuccessWithMeta(c, reviews, common.NewV2Meta(page, perPage, total))
}

// AddReview godoc
// @Summary 리뷰 작성
// @Tags marketplace
// @Param name path string true "플러그인 이름"
// @Success 201 {object} common.V2Response
// @Router /api/v2/marketplace/{name}/reviews [post]
func (h *MarketplaceHandler) AddReview(c *gin.Context) {
	name := c.Param("name")
	userID := getUserIDUint64(c)
	if userID == 0 {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	var req domain.PluginReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다", err)
		return
	}

	if err := h.svc.AddReview(name, userID, req); err != nil {
		common.V2ErrorResponse(c, http.StatusConflict, err.Error(), nil)
		return
	}
	common.V2Created(c, gin.H{"message": "리뷰가 등록되었습니다"})
}

// TrackDownload godoc
// @Summary 다운로드 기록
// @Tags marketplace
// @Param name path string true "플러그인 이름"
// @Success 200 {object} common.V2Response
// @Router /api/v2/marketplace/{name}/download [post]
func (h *MarketplaceHandler) TrackDownload(c *gin.Context) {
	name := c.Param("name")
	version := c.Query("version")
	ip := c.ClientIP()
	var userIDPtr *uint64
	if uid := getUserIDUint64(c); uid > 0 {
		userIDPtr = &uid
	}

	if err := h.svc.TrackDownload(name, version, ip, userIDPtr); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "다운로드 기록 실패", err)
		return
	}
	common.V2Success(c, gin.H{"message": "다운로드 기록 완료"})
}

// === Developer API ===

// RegisterDeveloper godoc
// @Summary 개발자 등록
// @Tags marketplace-developer
// @Success 201 {object} common.V2Response
// @Router /api/v2/marketplace/developers/register [post]
func (h *MarketplaceHandler) RegisterDeveloper(c *gin.Context) {
	userID := getUserIDUint64(c)
	if userID == 0 {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	var req domain.DeveloperRegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다", err)
		return
	}

	dev, err := h.svc.RegisterDeveloper(userID, req)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusConflict, err.Error(), nil)
		return
	}
	common.V2Created(c, dev)
}

// GetMyProfile godoc
// @Summary 내 개발자 프로필
// @Tags marketplace-developer
// @Success 200 {object} common.V2Response
// @Router /api/v2/marketplace/developers/me [get]
func (h *MarketplaceHandler) GetMyProfile(c *gin.Context) {
	userID := getUserIDUint64(c)
	if userID == 0 {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	dev, err := h.svc.GetDeveloperProfile(userID)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusNotFound, "개발자 등록이 필요합니다", err)
		return
	}
	common.V2Success(c, dev)
}

// SubmitPlugin godoc
// @Summary 플러그인 제출
// @Tags marketplace-developer
// @Success 201 {object} common.V2Response
// @Router /api/v2/marketplace/developers/submissions [post]
func (h *MarketplaceHandler) SubmitPlugin(c *gin.Context) {
	userID := getUserIDUint64(c)
	if userID == 0 {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	dev, err := h.svc.GetDeveloperProfile(userID)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusForbidden, "개발자 등록이 필요합니다", nil)
		return
	}

	var req domain.PluginSubmitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다", err)
		return
	}

	sub, err := h.svc.SubmitPlugin(dev.ID, req)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "플러그인 제출 실패", err)
		return
	}
	common.V2Created(c, sub)
}

// ListMySubmissions godoc
// @Summary 내 제출 목록
// @Tags marketplace-developer
// @Success 200 {object} common.V2Response
// @Router /api/v2/marketplace/developers/submissions [get]
func (h *MarketplaceHandler) ListMySubmissions(c *gin.Context) {
	userID := getUserIDUint64(c)
	if userID == 0 {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	dev, err := h.svc.GetDeveloperProfile(userID)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusForbidden, "개발자 등록이 필요합니다", nil)
		return
	}

	page, perPage := parsePage(c)
	subs, total, err := h.svc.ListMySubmissions(dev.ID, page, perPage)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "제출 목록 조회 실패", err)
		return
	}
	common.V2SuccessWithMeta(c, subs, common.NewV2Meta(page, perPage, total))
}

// === Admin API ===

// ListPendingSubmissions 관리자: 대기 중 제출 목록
func (h *MarketplaceHandler) ListPendingSubmissions(c *gin.Context) {
	page, perPage := parsePage(c)
	subs, total, err := h.svc.ListPendingSubmissions(page, perPage)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "대기 목록 조회 실패", err)
		return
	}
	common.V2SuccessWithMeta(c, subs, common.NewV2Meta(page, perPage, total))
}

// ReviewSubmission 관리자: 제출 승인/거절
func (h *MarketplaceHandler) ReviewSubmission(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 ID", err)
		return
	}

	var req struct {
		Approve bool   `json:"approve"`
		Note    string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다", err)
		return
	}

	reviewerID := getUserIDUint64(c)
	if err := h.svc.ReviewSubmission(id, reviewerID, req.Approve, req.Note); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	status := "승인"
	if !req.Approve {
		status = "거절"
	}
	common.V2Success(c, gin.H{"message": "제출이 " + status + "되었습니다"})
}

// === Helpers ===

func parsePage(c *gin.Context) (int, int) {
	page := 1
	if p, err := strconv.Atoi(c.Query("page")); err == nil && p > 0 {
		page = p
	}
	perPage := 20
	if l, err := strconv.Atoi(c.Query("per_page")); err == nil && l > 0 && l <= 100 {
		perPage = l
	}
	return page, perPage
}
