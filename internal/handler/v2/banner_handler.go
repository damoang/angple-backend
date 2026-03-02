package v2

import (
	"net/http"
	"strconv"

	"github.com/damoang/angple-backend/internal/common"
	v2domain "github.com/damoang/angple-backend/internal/domain/v2"
	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
	"github.com/gin-gonic/gin"
)

// BannerHandler handles banner API endpoints
type BannerHandler struct {
	bannerRepo v2repo.BannerRepository
}

// NewBannerHandler creates a new BannerHandler
func NewBannerHandler(bannerRepo v2repo.BannerRepository) *BannerHandler {
	return &BannerHandler{bannerRepo: bannerRepo}
}

// GetBanners handles GET /api/v1/banners?position=header|sidebar|content|footer
// TODO: v2 마이그레이션 - DB 재설계 후 /api/v2/banners로 전환
func (h *BannerHandler) GetBanners(c *gin.Context) {
	position := c.Query("position")

	banners, err := h.bannerRepo.FindActiveByPosition(position)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "배너 조회 실패", err)
		return
	}

	common.V2Success(c, banners)
}

// TrackClick handles GET /api/v1/banners/:id/click
// TODO: v2 마이그레이션 - DB 재설계 후 /api/v2/banners/:id/click로 전환
func (h *BannerHandler) TrackClick(c *gin.Context) {
	bannerID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 배너 ID", err)
		return
	}

	// Increment click count
	if err := h.bannerRepo.IncrementClickCount(bannerID); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "클릭 기록 실패", err)
		return
	}

	// Create click log
	ip := c.ClientIP()
	ua := c.GetHeader("User-Agent")
	referer := c.GetHeader("Referer")
	clickLog := &v2domain.BannerClickLog{
		BannerID:  bannerID,
		IPAddress: &ip,
		UserAgent: &ua,
		Referer:   &referer,
	}
	// Best-effort logging, don't fail the request
	_ = h.bannerRepo.CreateClickLog(clickLog)

	common.V2Success(c, gin.H{"message": "클릭 기록 완료"})
}
