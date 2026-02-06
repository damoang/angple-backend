package handler

import (
	"errors"
	"net/http"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/internal/service"
	"github.com/damoang/angple-backend/pkg/ginutil"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

var siteValidator = validator.New()

type SiteHandler struct {
	service *service.SiteService
}

func NewSiteHandler(service *service.SiteService) *SiteHandler {
	return &SiteHandler{service: service}
}

// ========================================
// Public Endpoints (인증 불필요)
// ========================================

// handleSiteRetrieval is a helper function to handle common site retrieval logic
func (h *SiteHandler) handleSiteRetrieval(c *gin.Context, site *domain.SiteResponse, err error) {
	if err != nil {
		if errors.Is(err, service.ErrSiteNotFound) {
			common.ErrorResponse(c, http.StatusNotFound, "Site not found", err)
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to retrieve site", err)
		return
	}

	common.SuccessResponse(c, map[string]interface{}{
		"site": site,
	}, nil)
}

// GetBySubdomain retrieves a site by subdomain
// GET /api/v2/sites/subdomain/:subdomain
func (h *SiteHandler) GetBySubdomain(c *gin.Context) {
	subdomain := c.Param("subdomain")
	if subdomain == "" {
		common.ErrorResponse(c, http.StatusBadRequest, "Subdomain is required", nil)
		return
	}

	site, err := h.service.GetBySubdomain(c.Request.Context(), subdomain)
	h.handleSiteRetrieval(c, site, err)
}

// GetByID retrieves a site by ID
// GET /api/v2/sites/:id
func (h *SiteHandler) GetByID(c *gin.Context) {
	siteID := c.Param("id")
	if siteID == "" {
		common.ErrorResponse(c, http.StatusBadRequest, "Site ID is required", nil)
		return
	}

	site, err := h.service.GetByID(c.Request.Context(), siteID)
	h.handleSiteRetrieval(c, site, err)
}

// Create creates a new site (for provisioning API)
// POST /api/v2/sites
func (h *SiteHandler) Create(c *gin.Context) {
	var req domain.CreateSiteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	// Validate request fields
	if err := siteValidator.Struct(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Validation failed", err)
		return
	}

	site, err := h.service.Create(c.Request.Context(), &req)
	if err != nil {
		if errors.Is(err, service.ErrInvalidSubdomain) {
			common.ErrorResponse(c, http.StatusBadRequest, "Invalid subdomain format", err)
			return
		} else if errors.Is(err, service.ErrSubdomainTaken) {
			common.ErrorResponse(c, http.StatusConflict, "Subdomain already taken", err)
			return
		} else if errors.Is(err, service.ErrInvalidPlan) {
			common.ErrorResponse(c, http.StatusBadRequest, "Invalid plan", err)
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to create site", err)
		return
	}

	c.JSON(http.StatusCreated, common.APIResponse{
		Data: map[string]interface{}{
			"site": site,
		},
	})
}

// ========================================
// Settings Endpoints
// ========================================

// GetSettings retrieves site settings
// GET /api/v2/sites/:id/settings
func (h *SiteHandler) GetSettings(c *gin.Context) {
	siteID := c.Param("id")
	if siteID == "" {
		common.ErrorResponse(c, http.StatusBadRequest, "Site ID is required", nil)
		return
	}

	settings, err := h.service.GetSettings(c.Request.Context(), siteID)
	if err != nil {
		if errors.Is(err, service.ErrSiteNotFound) {
			common.ErrorResponse(c, http.StatusNotFound, "Site not found", err)
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to retrieve settings", err)
		return
	}

	common.SuccessResponse(c, map[string]interface{}{
		"settings": settings,
	}, nil)
}

// UpdateSettings updates site settings
// PUT /api/v2/sites/:id/settings
func (h *SiteHandler) UpdateSettings(c *gin.Context) {
	siteID := c.Param("id")
	if siteID == "" {
		common.ErrorResponse(c, http.StatusBadRequest, "Site ID is required", nil)
		return
	}

	var req domain.UpdateSiteSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	// Check user permission (site owner/admin only)
	userID := middleware.GetUserID(c)
	if userID == "" {
		common.ErrorResponse(c, http.StatusUnauthorized, "Authentication required", nil)
		return
	}
	hasPermission, err := h.service.CheckUserPermission(c.Request.Context(), siteID, userID, "admin")
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to check permission", err)
		return
	}
	if !hasPermission {
		common.ErrorResponse(c, http.StatusForbidden, "Insufficient permission", nil)
		return
	}

	err = h.service.UpdateSettings(c.Request.Context(), siteID, &req)
	if err != nil {
		if errors.Is(err, service.ErrSiteNotFound) {
			common.ErrorResponse(c, http.StatusNotFound, "Site not found", err)
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to update settings", err)
		return
	}

	common.SuccessResponse(c, map[string]interface{}{
		"message": "Settings updated successfully",
	}, nil)
}

// ListActive retrieves all active sites (for admin dashboard)
// GET /api/v2/sites
func (h *SiteHandler) ListActive(c *gin.Context) {
	limit := ginutil.QueryInt(c, "limit", 20)
	offset := ginutil.QueryInt(c, "offset", 0)

	// 최대 100개로 제한
	if limit > 100 {
		limit = 100
	}

	sites, err := h.service.ListActive(c.Request.Context(), limit, offset)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to retrieve sites", err)
		return
	}

	common.SuccessResponse(c, map[string]interface{}{
		"sites": sites,
	}, &common.Meta{
		Limit: limit,
		Page:  offset / limit,
		Total: int64(len(sites)),
	})
}

// ========================================
// Subdomain Availability Check
// ========================================

// CheckSubdomainAvailability checks if subdomain is available
// GET /api/v2/sites/check-subdomain/:subdomain
func (h *SiteHandler) CheckSubdomainAvailability(c *gin.Context) {
	subdomain := c.Param("subdomain")
	if subdomain == "" {
		common.ErrorResponse(c, http.StatusBadRequest, "Subdomain is required", nil)
		return
	}

	// 먼저 서비스 레이어의 검증 로직 사용
	if !h.service.ValidateSubdomain(subdomain) {
		common.SuccessResponse(c, map[string]interface{}{
			"available": false,
			"reason":    "Invalid subdomain format",
		}, nil)
		return
	}

	// DB에서 중복 체크
	site, err := h.service.GetBySubdomain(c.Request.Context(), subdomain)
	if err != nil && !errors.Is(err, service.ErrSiteNotFound) {
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to check subdomain", err)
		return
	}

	available := (site == nil || errors.Is(err, service.ErrSiteNotFound))
	reason := ""
	if !available {
		reason = "Subdomain already taken"
	}

	data := map[string]interface{}{
		"available": available,
	}
	if reason != "" {
		data["reason"] = reason
	}

	common.SuccessResponse(c, data, nil)
}
