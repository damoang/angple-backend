package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/plugins/advertising/domain"
	"github.com/damoang/angple-backend/internal/plugins/advertising/service"
	"github.com/gin-gonic/gin"
)

// AdminHandler 관리자 광고 API 핸들러
type AdminHandler struct {
	adUnitService service.AdUnitService
	bannerService service.BannerService
}

// NewAdminHandler 관리자 핸들러 생성자
func NewAdminHandler(
	adUnitService service.AdUnitService,
	bannerService service.BannerService,
) *AdminHandler {
	return &AdminHandler{
		adUnitService: adUnitService,
		bannerService: bannerService,
	}
}

// ============ Ad Unit Handlers ============

// ListAdUnits godoc
// @Summary      광고 단위 목록 조회
// @Description  모든 광고 단위를 조회합니다
// @Tags         advertising-admin
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  common.APIResponse{data=[]domain.AdUnitResponse}
// @Failure      401  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/advertising/admin/units [get]
func (h *AdminHandler) ListAdUnits(c *gin.Context) {
	units, err := h.adUnitService.ListAdUnits()
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to list ad units", err)
		return
	}

	common.SuccessResponse(c, units, nil)
}

// GetAdUnit godoc
// @Summary      광고 단위 상세 조회
// @Description  특정 광고 단위의 상세 정보를 조회합니다
// @Tags         advertising-admin
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "광고 단위 ID"
// @Success      200  {object}  common.APIResponse{data=domain.AdUnitResponse}
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      404  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/advertising/admin/units/{id} [get]
func (h *AdminHandler) GetAdUnit(c *gin.Context) {
	id, err := h.parseID(c, "id")
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid ad unit ID", err)
		return
	}

	unit, err := h.adUnitService.GetAdUnit(id)
	if err != nil {
		if errors.Is(err, service.ErrAdUnitNotFound) {
			common.ErrorResponse(c, http.StatusNotFound, "Ad unit not found", err)
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to get ad unit", err)
		return
	}

	common.SuccessResponse(c, unit, nil)
}

// CreateAdUnit godoc
// @Summary      광고 단위 생성
// @Description  새 광고 단위를 생성합니다
// @Tags         advertising-admin
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request  body      domain.CreateAdUnitRequest  true  "광고 단위 생성 요청"
// @Success      201  {object}  common.APIResponse{data=domain.AdUnitResponse}
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/advertising/admin/units [post]
func (h *AdminHandler) CreateAdUnit(c *gin.Context) {
	var req domain.CreateAdUnitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	unit, err := h.adUnitService.CreateAdUnit(&req)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to create ad unit", err)
		return
	}

	c.JSON(http.StatusCreated, common.APIResponse{Data: unit})
}

// UpdateAdUnit godoc
// @Summary      광고 단위 수정
// @Description  기존 광고 단위를 수정합니다
// @Tags         advertising-admin
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path      int                        true  "광고 단위 ID"
// @Param        request  body      domain.UpdateAdUnitRequest true  "광고 단위 수정 요청"
// @Success      200  {object}  common.APIResponse{data=domain.AdUnitResponse}
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      404  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/advertising/admin/units/{id} [put]
func (h *AdminHandler) UpdateAdUnit(c *gin.Context) { //nolint:dupl
	id, err := h.parseID(c, "id")
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid ad unit ID", err)
		return
	}

	var req domain.UpdateAdUnitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	unit, err := h.adUnitService.UpdateAdUnit(id, &req)
	if err != nil {
		if errors.Is(err, service.ErrAdUnitNotFound) {
			common.ErrorResponse(c, http.StatusNotFound, "Ad unit not found", err)
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to update ad unit", err)
		return
	}

	common.SuccessResponse(c, unit, nil)
}

// DeleteAdUnit godoc
// @Summary      광고 단위 삭제
// @Description  광고 단위를 삭제합니다
// @Tags         advertising-admin
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "광고 단위 ID"
// @Success      200  {object}  common.APIResponse
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      404  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/advertising/admin/units/{id} [delete]
func (h *AdminHandler) DeleteAdUnit(c *gin.Context) {
	id, err := h.parseID(c, "id")
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid ad unit ID", err)
		return
	}

	if err := h.adUnitService.DeleteAdUnit(id); err != nil {
		if errors.Is(err, service.ErrAdUnitNotFound) {
			common.ErrorResponse(c, http.StatusNotFound, "Ad unit not found", err)
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to delete ad unit", err)
		return
	}

	common.SuccessResponse(c, map[string]string{"message": "Ad unit deleted successfully"}, nil)
}

// ============ Banner Handlers ============

// ListBanners godoc
// @Summary      축하 배너 목록 조회
// @Description  모든 축하 배너를 조회합니다
// @Tags         advertising-admin
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        active_only query   bool  false  "활성화된 배너만 조회"
// @Success      200  {object}  common.APIResponse{data=[]domain.CelebrationBannerResponse}
// @Failure      401  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/advertising/admin/banners [get]
func (h *AdminHandler) ListBanners(c *gin.Context) {
	activeOnly := c.Query("active_only") == "true"

	banners, err := h.bannerService.ListBanners(activeOnly)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to list banners", err)
		return
	}

	common.SuccessResponse(c, banners, nil)
}

// GetBanner godoc
// @Summary      축하 배너 상세 조회
// @Description  특정 축하 배너의 상세 정보를 조회합니다
// @Tags         advertising-admin
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "배너 ID"
// @Success      200  {object}  common.APIResponse{data=domain.CelebrationBannerResponse}
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      404  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/advertising/admin/banners/{id} [get]
func (h *AdminHandler) GetBanner(c *gin.Context) {
	id, err := h.parseID(c, "id")
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid banner ID", err)
		return
	}

	banner, err := h.bannerService.GetBanner(id)
	if err != nil {
		if errors.Is(err, service.ErrBannerNotFound) {
			common.ErrorResponse(c, http.StatusNotFound, "Banner not found", err)
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to get banner", err)
		return
	}

	common.SuccessResponse(c, banner, nil)
}

// CreateBanner godoc
// @Summary      축하 배너 생성
// @Description  새 축하 배너를 생성합니다
// @Tags         advertising-admin
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request  body      domain.CreateBannerRequest  true  "배너 생성 요청"
// @Success      201  {object}  common.APIResponse{data=domain.CelebrationBannerResponse}
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/advertising/admin/banners [post]
func (h *AdminHandler) CreateBanner(c *gin.Context) {
	var req domain.CreateBannerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	banner, err := h.bannerService.CreateBanner(&req)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to create banner", err)
		return
	}

	c.JSON(http.StatusCreated, common.APIResponse{Data: banner})
}

// UpdateBanner godoc
// @Summary      축하 배너 수정
// @Description  기존 축하 배너를 수정합니다
// @Tags         advertising-admin
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path      int                        true  "배너 ID"
// @Param        request  body      domain.UpdateBannerRequest true  "배너 수정 요청"
// @Success      200  {object}  common.APIResponse{data=domain.CelebrationBannerResponse}
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      404  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/advertising/admin/banners/{id} [put]
func (h *AdminHandler) UpdateBanner(c *gin.Context) { //nolint:dupl
	id, err := h.parseID(c, "id")
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid banner ID", err)
		return
	}

	var req domain.UpdateBannerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	banner, err := h.bannerService.UpdateBanner(id, &req)
	if err != nil {
		if errors.Is(err, service.ErrBannerNotFound) {
			common.ErrorResponse(c, http.StatusNotFound, "Banner not found", err)
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to update banner", err)
		return
	}

	common.SuccessResponse(c, banner, nil)
}

// DeleteBanner godoc
// @Summary      축하 배너 삭제
// @Description  축하 배너를 삭제합니다
// @Tags         advertising-admin
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "배너 ID"
// @Success      200  {object}  common.APIResponse
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      404  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/advertising/admin/banners/{id} [delete]
func (h *AdminHandler) DeleteBanner(c *gin.Context) {
	id, err := h.parseID(c, "id")
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid banner ID", err)
		return
	}

	if err := h.bannerService.DeleteBanner(id); err != nil {
		if errors.Is(err, service.ErrBannerNotFound) {
			common.ErrorResponse(c, http.StatusNotFound, "Banner not found", err)
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to delete banner", err)
		return
	}

	common.SuccessResponse(c, map[string]string{"message": "Banner deleted successfully"}, nil)
}

// parseID URL 파라미터에서 ID 파싱
func (h *AdminHandler) parseID(c *gin.Context, param string) (uint64, error) {
	idStr := c.Param(param)
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		return 0, err
	}
	return id, nil
}
