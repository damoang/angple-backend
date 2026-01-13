package handler

import (
	"errors"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/service"
	"github.com/gofiber/fiber/v2"
)

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
func (h *SiteHandler) handleSiteRetrieval(c *fiber.Ctx, site *domain.SiteResponse, err error) error {
	if err != nil {
		if errors.Is(err, service.ErrSiteNotFound) {
			return common.ErrorResponse(c, fiber.StatusNotFound, "Site not found", err)
		}
		return common.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to retrieve site", err)
	}

	return common.SuccessResponse(c, map[string]interface{}{
		"site": site,
	}, nil)
}

// GetBySubdomain retrieves a site by subdomain
// GET /api/v2/sites/subdomain/:subdomain
func (h *SiteHandler) GetBySubdomain(c *fiber.Ctx) error {
	subdomain := c.Params("subdomain")
	if subdomain == "" {
		return common.ErrorResponse(c, fiber.StatusBadRequest, "Subdomain is required", nil)
	}

	site, err := h.service.GetBySubdomain(c.Context(), subdomain)
	return h.handleSiteRetrieval(c, site, err)
}

// GetByID retrieves a site by ID
// GET /api/v2/sites/:id
func (h *SiteHandler) GetByID(c *fiber.Ctx) error {
	siteID := c.Params("id")
	if siteID == "" {
		return common.ErrorResponse(c, fiber.StatusBadRequest, "Site ID is required", nil)
	}

	site, err := h.service.GetByID(c.Context(), siteID)
	return h.handleSiteRetrieval(c, site, err)
}

// Create creates a new site (for provisioning API)
// POST /api/v2/sites
func (h *SiteHandler) Create(c *fiber.Ctx) error {
	var req domain.CreateSiteRequest
	if err := c.BodyParser(&req); err != nil {
		return common.ErrorResponse(c, fiber.StatusBadRequest, "Invalid request body", err)
	}

	// TODO: 향후 validator 적용 필요
	// if err := validate.Struct(&req); err != nil { ... }

	site, err := h.service.Create(c.Context(), &req)
	if err != nil {
		if errors.Is(err, service.ErrInvalidSubdomain) {
			return common.ErrorResponse(c, fiber.StatusBadRequest, "Invalid subdomain format", err)
		} else if errors.Is(err, service.ErrSubdomainTaken) {
			return common.ErrorResponse(c, fiber.StatusConflict, "Subdomain already taken", err)
		} else if errors.Is(err, service.ErrInvalidPlan) {
			return common.ErrorResponse(c, fiber.StatusBadRequest, "Invalid plan", err)
		}
		return common.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to create site", err)
	}

	return c.Status(fiber.StatusCreated).JSON(common.APIResponse{
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
func (h *SiteHandler) GetSettings(c *fiber.Ctx) error {
	siteID := c.Params("id")
	if siteID == "" {
		return common.ErrorResponse(c, fiber.StatusBadRequest, "Site ID is required", nil)
	}

	settings, err := h.service.GetSettings(c.Context(), siteID)
	if err != nil {
		if errors.Is(err, service.ErrSiteNotFound) {
			return common.ErrorResponse(c, fiber.StatusNotFound, "Site not found", err)
		}
		return common.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to retrieve settings", err)
	}

	return common.SuccessResponse(c, map[string]interface{}{
		"settings": settings,
	}, nil)
}

// UpdateSettings updates site settings
// PUT /api/v2/sites/:id/settings
func (h *SiteHandler) UpdateSettings(c *fiber.Ctx) error {
	siteID := c.Params("id")
	if siteID == "" {
		return common.ErrorResponse(c, fiber.StatusBadRequest, "Site ID is required", nil)
	}

	var req domain.UpdateSiteSettingsRequest
	if err := c.BodyParser(&req); err != nil {
		return common.ErrorResponse(c, fiber.StatusBadRequest, "Invalid request body", err)
	}

	// TODO: 향후 인증 추가 필요 (site owner/admin만 수정 가능)
	// userID := c.Locals("user_id").(string)
	// hasPermission, _ := h.service.CheckUserPermission(c.Context(), siteID, userID, "admin")
	// if !hasPermission { return Forbidden }

	err := h.service.UpdateSettings(c.Context(), siteID, &req)
	if err != nil {
		if errors.Is(err, service.ErrSiteNotFound) {
			return common.ErrorResponse(c, fiber.StatusNotFound, "Site not found", err)
		}
		return common.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to update settings", err)
	}

	return common.SuccessResponse(c, map[string]interface{}{
		"message": "Settings updated successfully",
	}, nil)
}

// ListActive retrieves all active sites (for admin dashboard)
// GET /api/v2/sites
func (h *SiteHandler) ListActive(c *fiber.Ctx) error {
	limit := c.QueryInt("limit", 20)
	offset := c.QueryInt("offset", 0)

	// 최대 100개로 제한
	if limit > 100 {
		limit = 100
	}

	sites, err := h.service.ListActive(c.Context(), limit, offset)
	if err != nil {
		return common.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to retrieve sites", err)
	}

	return common.SuccessResponse(c, map[string]interface{}{
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
func (h *SiteHandler) CheckSubdomainAvailability(c *fiber.Ctx) error {
	subdomain := c.Params("subdomain")
	if subdomain == "" {
		return common.ErrorResponse(c, fiber.StatusBadRequest, "Subdomain is required", nil)
	}

	// 먼저 서비스 레이어의 검증 로직 사용
	if !h.service.ValidateSubdomain(subdomain) {
		return common.SuccessResponse(c, map[string]interface{}{
			"available": false,
			"reason":    "Invalid subdomain format",
		}, nil)
	}

	// DB에서 중복 체크
	site, err := h.service.GetBySubdomain(c.Context(), subdomain)
	if err != nil && !errors.Is(err, service.ErrSiteNotFound) {
		return common.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to check subdomain", err)
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

	return common.SuccessResponse(c, data, nil)
}
