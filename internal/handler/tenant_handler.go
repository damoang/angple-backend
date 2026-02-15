package handler

import (
	"net/http"
	"strconv"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/internal/service"
	"github.com/gin-gonic/gin"
)

func parseIntQuery(c *gin.Context, key string, defaultVal int) int {
	if v, err := strconv.Atoi(c.Query(key)); err == nil && v > 0 {
		return v
	}
	return defaultVal
}

// TenantHandler handles tenant management admin API
type TenantHandler struct {
	tenantService *service.TenantService
}

// NewTenantHandler creates a new TenantHandler
func NewTenantHandler(tenantService *service.TenantService) *TenantHandler {
	return &TenantHandler{tenantService: tenantService}
}

// ListTenants godoc
// @Summary 테넌트 목록 (관리자)
// @Tags admin-tenants
// @Param page query int false "페이지" default(1)
// @Param per_page query int false "페이지당 항목" default(20)
// @Param status query string false "상태 필터 (active, suspended, all)" default(all)
// @Success 200 {object} common.V2Response
// @Router /api/v2/admin/tenants [get]
func (h *TenantHandler) ListTenants(c *gin.Context) {
	page := parseIntQuery(c, "page", 1)
	perPage := parseIntQuery(c, "per_page", 20)
	status := c.DefaultQuery("status", "all")

	tenants, total, err := h.tenantService.ListTenants(c.Request.Context(), page, perPage, status)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "테넌트 목록 조회 실패", err)
		return
	}
	common.V2SuccessWithMeta(c, tenants, common.NewV2Meta(page, perPage, total))
}

// GetTenant godoc
// @Summary 테넌트 상세 (관리자)
// @Tags admin-tenants
// @Param id path string true "사이트 ID"
// @Success 200 {object} common.V2Response
// @Router /api/v2/admin/tenants/{id} [get]
func (h *TenantHandler) GetTenant(c *gin.Context) {
	siteID := c.Param("id")
	tenant, err := h.tenantService.GetTenantDetail(c.Request.Context(), siteID)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusNotFound, "테넌트를 찾을 수 없습니다", err)
		return
	}
	common.V2Success(c, tenant)
}

// SuspendTenant godoc
// @Summary 테넌트 정지 (관리자)
// @Tags admin-tenants
// @Param id path string true "사이트 ID"
// @Success 200 {object} common.V2Response
// @Router /api/v2/admin/tenants/{id}/suspend [post]
func (h *TenantHandler) SuspendTenant(c *gin.Context) {
	siteID := c.Param("id")
	var req struct {
		Reason string `json:"reason"`
	}
	// Reason is optional; ignore bind errors (body may be empty)
	_ = c.ShouldBindJSON(&req) //nolint:errcheck // optional body

	if err := h.tenantService.SuspendTenant(c.Request.Context(), siteID, req.Reason); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	common.V2Success(c, gin.H{"message": "테넌트가 정지되었습니다"})
}

// UnsuspendTenant godoc
// @Summary 테넌트 정지 해제 (관리자)
// @Tags admin-tenants
// @Param id path string true "사이트 ID"
// @Success 200 {object} common.V2Response
// @Router /api/v2/admin/tenants/{id}/unsuspend [post]
func (h *TenantHandler) UnsuspendTenant(c *gin.Context) {
	siteID := c.Param("id")
	if err := h.tenantService.UnsuspendTenant(c.Request.Context(), siteID); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	common.V2Success(c, gin.H{"message": "테넌트 정지가 해제되었습니다"})
}

// ChangePlan godoc
// @Summary 테넌트 플랜 변경 (관리자)
// @Tags admin-tenants
// @Param id path string true "사이트 ID"
// @Success 200 {object} common.V2Response
// @Router /api/v2/admin/tenants/{id}/plan [put]
func (h *TenantHandler) ChangePlan(c *gin.Context) {
	siteID := c.Param("id")
	var req struct {
		Plan string `json:"plan" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다", err)
		return
	}

	if err := h.tenantService.ChangePlan(c.Request.Context(), siteID, req.Plan); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	common.V2Success(c, gin.H{"message": "플랜이 변경되었습니다", "plan": req.Plan})
}

// GetUsage godoc
// @Summary 테넌트 사용량 조회
// @Tags admin-tenants
// @Param id path string true "사이트 ID"
// @Success 200 {object} common.V2Response
// @Router /api/v2/admin/tenants/{id}/usage [get]
func (h *TenantHandler) GetUsage(c *gin.Context) {
	siteID := c.Param("id")
	days := parseIntQuery(c, "days", 30)

	usage, err := h.tenantService.GetUsage(c.Request.Context(), siteID, days)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "사용량 조회 실패", err)
		return
	}
	common.V2Success(c, usage)
}

// GetPlanLimits godoc
// @Summary 플랜별 리소스 제한 조회
// @Tags admin-tenants
// @Success 200 {object} common.V2Response
// @Router /api/v2/admin/tenants/plans [get]
func (h *TenantHandler) GetPlanLimits(c *gin.Context) {
	plans := map[string]middleware.PlanLimits{
		"free":       middleware.GetPlanLimits("free"),
		"pro":        middleware.GetPlanLimits("pro"),
		"business":   middleware.GetPlanLimits("business"),
		"enterprise": middleware.GetPlanLimits("enterprise"),
	}
	common.V2Success(c, plans)
}
