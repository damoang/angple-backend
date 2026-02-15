package handler

import (
	"net/http"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/plugins/advertising/service"
	"github.com/gin-gonic/gin"
)

// PublicHandler 공개 광고 API 핸들러
type PublicHandler struct {
	gamService     service.GAMService
	adsenseService service.AdsenseService
	adUnitService  service.AdUnitService
	bannerService  service.BannerService
}

// NewPublicHandler 공개 핸들러 생성자
func NewPublicHandler(
	gamService service.GAMService,
	adsenseService service.AdsenseService,
	adUnitService service.AdUnitService,
	bannerService service.BannerService,
) *PublicHandler {
	return &PublicHandler{
		gamService:     gamService,
		adsenseService: adsenseService,
		adUnitService:  adUnitService,
		bannerService:  bannerService,
	}
}

// GetGAMConfig godoc
// @Summary      GAM 전역 설정 조회
// @Description  Google Ad Manager 전역 설정을 조회합니다
// @Tags         advertising-public
// @Accept       json
// @Produce      json
// @Success      200  {object}  common.APIResponse{data=domain.GAMConfigResponse}
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/advertising/gam/config [get]
func (h *PublicHandler) GetGAMConfig(c *gin.Context) {
	config, err := h.gamService.GetConfig()
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to get GAM config", err)
		return
	}

	common.SuccessResponse(c, config, nil)
}

// GetAdsenseConfig godoc
// @Summary      AdSense 전역 설정 조회
// @Description  AdSense 전역 설정을 조회합니다
// @Tags         advertising-public
// @Accept       json
// @Produce      json
// @Success      200  {object}  common.APIResponse{data=domain.AdsenseConfigResponse}
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/advertising/adsense/config [get]
func (h *PublicHandler) GetAdsenseConfig(c *gin.Context) {
	config, err := h.adsenseService.GetConfig()
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to get AdSense config", err)
		return
	}

	common.SuccessResponse(c, config, nil)
}

// GetAdByPosition godoc
// @Summary      위치별 광고 설정 조회
// @Description  특정 위치의 광고 설정(GAM + AdSense fallback)을 조회합니다
// @Tags         advertising-public
// @Accept       json
// @Produce      json
// @Param        position   path      string  true   "광고 위치"
// @Param        session_key query   string  false  "세션 키 (로테이션용)"
// @Success      200  {object}  common.APIResponse{data=domain.AdPositionResponse}
// @Failure      404  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/advertising/units/{position} [get]
func (h *PublicHandler) GetAdByPosition(c *gin.Context) {
	position := c.Param("position")
	if position == "" {
		common.ErrorResponse(c, http.StatusBadRequest, "Position is required", nil)
		return
	}

	// 세션 키: JWT jti 또는 쿼리 파라미터
	sessionKey := c.Query("session_key")
	if sessionKey == "" {
		// JWT에서 jti 추출 시도
		if jti, exists := c.Get("jti"); exists {
			if s, ok := jti.(string); ok {
				sessionKey = s
			}
		}
	}

	config, err := h.adUnitService.GetAdConfig(position, sessionKey)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to get ad config", err)
		return
	}

	common.SuccessResponse(c, config, nil)
}

// GetTodayBanners godoc
// @Summary      오늘의 축하 배너 조회
// @Description  오늘 날짜의 활성화된 축하 배너를 조회합니다
// @Tags         advertising-public
// @Accept       json
// @Produce      json
// @Success      200  {object}  common.APIResponse{data=[]domain.CelebrationBannerResponse}
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/advertising/banners/today [get]
func (h *PublicHandler) GetTodayBanners(c *gin.Context) {
	banners, err := h.bannerService.GetTodayBanners()
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to get today's banners", err)
		return
	}

	common.SuccessResponse(c, banners, nil)
}

// GetBannersByDate godoc
// @Summary      날짜별 축하 배너 조회
// @Description  특정 날짜의 활성화된 축하 배너를 조회합니다
// @Tags         advertising-public
// @Accept       json
// @Produce      json
// @Param        date   path      string  true   "날짜 (YYYY-MM-DD)"
// @Success      200  {object}  common.APIResponse{data=[]domain.CelebrationBannerResponse}
// @Failure      400  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/advertising/banners/date/{date} [get]
func (h *PublicHandler) GetBannersByDate(c *gin.Context) {
	dateStr := c.Param("date")
	if dateStr == "" {
		common.ErrorResponse(c, http.StatusBadRequest, "Date is required", nil)
		return
	}

	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid date format, expected YYYY-MM-DD", err)
		return
	}

	banners, err := h.bannerService.GetBannersByDate(date)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to get banners", err)
		return
	}

	common.SuccessResponse(c, banners, nil)
}

// GetRotationIndex godoc
// @Summary      로테이션 인덱스 조회
// @Description  세션 키 기반 광고 로테이션 인덱스를 조회합니다
// @Tags         advertising-public
// @Accept       json
// @Produce      json
// @Param        session_key query   string  false  "세션 키"
// @Param        max_slots   query   int     false  "최대 슬롯 수 (기본값: 8)"
// @Success      200  {object}  common.APIResponse{data=map[string]int}
// @Router       /plugins/advertising/rotation-index [get]
func (h *PublicHandler) GetRotationIndex(c *gin.Context) {
	sessionKey := c.Query("session_key")
	if sessionKey == "" {
		if jti, exists := c.Get("jti"); exists {
			if s, ok := jti.(string); ok {
				sessionKey = s
			}
		}
	}

	maxSlots := 8
	if maxSlotsQuery := c.Query("max_slots"); maxSlotsQuery != "" {
		if parsed, err := parseInt(maxSlotsQuery); err == nil && parsed > 0 {
			maxSlots = parsed
		}
	}

	index := h.adsenseService.GetRotationIndex(sessionKey, maxSlots)

	common.SuccessResponse(c, map[string]int{
		"rotation_index": index,
		"max_slots":      maxSlots,
	}, nil)
}

// parseInt 문자열을 정수로 변환
func parseInt(s string) (int, error) {
	var result int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, nil
		}
		result = result*10 + int(c-'0')
	}
	return result, nil
}
