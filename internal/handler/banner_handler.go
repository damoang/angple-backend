package handler

import (
	"net/http"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/service"
	"github.com/damoang/angple-backend/pkg/ginutil"
	"github.com/gin-gonic/gin"
)

// BannerHandler handles HTTP requests for banners
type BannerHandler struct {
	service service.BannerService
}

// NewBannerHandler creates a new BannerHandler
func NewBannerHandler(service service.BannerService) *BannerHandler {
	return &BannerHandler{service: service}
}

// ListBanners godoc
// @Summary      배너 목록 조회
// @Description  활성 배너 목록을 위치별로 조회합니다
// @Tags         banners
// @Accept       json
// @Produce      json
// @Param        position  query     string  false  "배너 위치 (header, sidebar, content, footer)"
// @Success      200  {object}  common.APIResponse{data=domain.BannerListResponse}
// @Failure      500  {object}  common.APIResponse
// @Router       /banners [get]
func (h *BannerHandler) ListBanners(c *gin.Context) {
	position := c.Query("position")

	if position != "" {
		// Get banners by position
		data, err := h.service.GetBannersByPosition(domain.BannerPosition(position))
		if err != nil {
			common.ErrorResponse(c, 500, "Failed to fetch banners", err)
			return
		}
		common.SuccessResponse(c, data, nil)
		return
	}

	// Get all active banners
	data, err := h.service.GetActiveBanners()
	if err != nil {
		common.ErrorResponse(c, 500, "Failed to fetch banners", err)
		return
	}

	common.SuccessResponse(c, data, nil)
}

// GetBanner godoc
// @Summary      배너 상세 조회
// @Description  배너 상세 정보를 조회합니다
// @Tags         banners
// @Accept       json
// @Produce      json
// @Param        id  path  int  true  "배너 ID"
// @Success      200  {object}  common.APIResponse{data=domain.BannerResponse}
// @Failure      400  {object}  common.APIResponse
// @Failure      404  {object}  common.APIResponse
// @Router       /banners/{id} [get]
func (h *BannerHandler) GetBanner(c *gin.Context) {
	id, err := ginutil.ParamInt64(c, "id")
	if err != nil {
		common.ErrorResponse(c, 400, "Invalid banner ID", err)
		return
	}

	data, err := h.service.GetBannerByID(id)
	if err != nil {
		common.ErrorResponse(c, 404, "Banner not found", err)
		return
	}

	common.SuccessResponse(c, data, nil)
}

// TrackClick godoc
// @Summary      배너 클릭 트래킹
// @Description  배너 클릭을 트래킹합니다
// @Tags         banners
// @Accept       json
// @Produce      json
// @Param        id  path  int  true  "배너 ID"
// @Success      200  {object}  common.APIResponse
// @Failure      400  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /banners/{id}/click [get]
func (h *BannerHandler) TrackClick(c *gin.Context) {
	id, err := ginutil.ParamInt64(c, "id")
	if err != nil {
		common.ErrorResponse(c, 400, "Invalid banner ID", err)
		return
	}

	// Create click request with client info
	req := &domain.BannerClickRequest{
		IPAddress: c.ClientIP(),
		UserAgent: c.GetHeader("User-Agent"),
		Referer:   c.GetHeader("Referer"),
	}

	// Get member ID if logged in
	if memberID, exists := c.Get("damoang_user_id"); exists {
		if str, ok := memberID.(string); ok {
			req.MemberID = str
		}
	}

	if err := h.service.TrackClick(id, req); err != nil {
		common.ErrorResponse(c, 500, "Failed to track click", err)
		return
	}

	// Get banner info to redirect to link URL
	banner, err := h.service.GetBannerByID(id)
	if err != nil {
		common.ErrorResponse(c, 404, "Banner not found", err)
		return
	}

	if banner.LinkURL != "" {
		c.Redirect(http.StatusFound, banner.LinkURL)
		return
	}

	common.SuccessResponse(c, gin.H{"message": "Click tracked"}, nil)
}

// TrackView godoc
// @Summary      배너 노출 트래킹
// @Description  배너 노출을 트래킹합니다
// @Tags         banners
// @Accept       json
// @Produce      json
// @Param        id  path  int  true  "배너 ID"
// @Success      200  {object}  common.APIResponse
// @Failure      400  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /banners/{id}/view [post]
func (h *BannerHandler) TrackView(c *gin.Context) {
	id, err := ginutil.ParamInt64(c, "id")
	if err != nil {
		common.ErrorResponse(c, 400, "Invalid banner ID", err)
		return
	}

	if err := h.service.TrackView(id); err != nil {
		common.ErrorResponse(c, 500, "Failed to track view", err)
		return
	}

	common.SuccessResponse(c, gin.H{"message": "View tracked"}, nil)
}

// ============= Admin Endpoints =============

// ListAllBanners godoc
// @Summary      모든 배너 목록 조회 (관리자)
// @Description  활성/비활성 포함 모든 배너 목록을 조회합니다
// @Tags         banners
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  common.APIResponse{data=[]domain.BannerResponse}
// @Failure      401  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /admin/banners [get]
func (h *BannerHandler) ListAllBanners(c *gin.Context) {
	data, err := h.service.GetAllBanners()
	if err != nil {
		common.ErrorResponse(c, 500, "Failed to fetch banners", err)
		return
	}

	common.SuccessResponse(c, data, nil)
}

// CreateBanner godoc
// @Summary      배너 추가 (관리자)
// @Description  새 배너를 추가합니다
// @Tags         banners
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request  body      domain.CreateBannerRequest  true  "배너 추가 요청"
// @Success      201  {object}  common.APIResponse{data=domain.BannerResponse}
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /admin/banners [post]
func (h *BannerHandler) CreateBanner(c *gin.Context) {
	var req domain.CreateBannerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, 400, "Invalid request body", err)
		return
	}

	data, err := h.service.CreateBanner(&req)
	if err != nil {
		common.ErrorResponse(c, 500, "Failed to create banner", err)
		return
	}

	c.JSON(http.StatusCreated, common.APIResponse{Data: data})
}

// UpdateBanner godoc
// @Summary      배너 수정 (관리자)
// @Description  배너 정보를 수정합니다
// @Tags         banners
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path      int                         true  "배너 ID"
// @Param        request  body      domain.UpdateBannerRequest  true  "배너 수정 요청"
// @Success      200  {object}  common.APIResponse{data=domain.BannerResponse}
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      404  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /admin/banners/{id} [put]
func (h *BannerHandler) UpdateBanner(c *gin.Context) {
	id, err := ginutil.ParamInt64(c, "id")
	if err != nil {
		common.ErrorResponse(c, 400, "Invalid banner ID", err)
		return
	}

	var req domain.UpdateBannerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, 400, "Invalid request body", err)
		return
	}

	data, err := h.service.UpdateBanner(id, &req)
	if err != nil {
		common.ErrorResponse(c, 500, "Failed to update banner", err)
		return
	}

	common.SuccessResponse(c, data, nil)
}

// DeleteBanner godoc
// @Summary      배너 삭제 (관리자)
// @Description  배너를 삭제합니다
// @Tags         banners
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  int  true  "배너 ID"
// @Success      200  {object}  common.APIResponse
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /admin/banners/{id} [delete]
func (h *BannerHandler) DeleteBanner(c *gin.Context) {
	id, err := ginutil.ParamInt64(c, "id")
	if err != nil {
		common.ErrorResponse(c, 400, "Invalid banner ID", err)
		return
	}

	if err := h.service.DeleteBanner(id); err != nil {
		common.ErrorResponse(c, 500, "Failed to delete banner", err)
		return
	}

	common.SuccessResponse(c, gin.H{"message": "Banner deleted successfully"}, nil)
}

// GetBannerStats godoc
// @Summary      배너 통계 조회 (관리자)
// @Description  배너의 클릭/노출 통계를 조회합니다
// @Tags         banners
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  int  true  "배너 ID"
// @Success      200  {object}  common.APIResponse{data=domain.BannerStatsResponse}
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      404  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /admin/banners/{id}/stats [get]
func (h *BannerHandler) GetBannerStats(c *gin.Context) {
	id, err := ginutil.ParamInt64(c, "id")
	if err != nil {
		common.ErrorResponse(c, 400, "Invalid banner ID", err)
		return
	}

	data, err := h.service.GetBannerStats(id)
	if err != nil {
		common.ErrorResponse(c, 500, "Failed to fetch banner stats", err)
		return
	}

	common.SuccessResponse(c, data, nil)
}
