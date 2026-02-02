package handler

import (
	"net/http"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/service"
	"github.com/gin-gonic/gin"
)

// ProvisioningHandler handles SaaS provisioning and subscription APIs
type ProvisioningHandler struct {
	provisioningSvc *service.ProvisioningService
}

// NewProvisioningHandler creates a new ProvisioningHandler
func NewProvisioningHandler(provisioningSvc *service.ProvisioningService) *ProvisioningHandler {
	return &ProvisioningHandler{provisioningSvc: provisioningSvc}
}

// ProvisionCommunity godoc
// @Summary 원클릭 커뮤니티 생성
// @Tags saas
// @Accept json
// @Produce json
// @Param body body domain.ProvisionRequest true "생성 정보"
// @Success 201 {object} common.V2Response
// @Router /api/v2/saas/communities [post]
func (h *ProvisioningHandler) ProvisionCommunity(c *gin.Context) {
	var req domain.ProvisionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다", err)
		return
	}

	result, err := h.provisioningSvc.ProvisionCommunity(c.Request.Context(), &req)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	common.V2Created(c, result)
}

// DeleteCommunity godoc
// @Summary 커뮤니티 삭제 (비활성화)
// @Tags saas
// @Param id path string true "사이트 ID"
// @Success 200 {object} common.V2Response
// @Router /api/v2/saas/communities/{id} [delete]
func (h *ProvisioningHandler) DeleteCommunity(c *gin.Context) {
	siteID := c.Param("id")
	if err := h.provisioningSvc.DeleteCommunity(c.Request.Context(), siteID); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	common.V2Success(c, gin.H{"message": "커뮤니티가 비활성화되었습니다"})
}

// GetSubscription godoc
// @Summary 구독 정보 조회
// @Tags saas
// @Param id path string true "사이트 ID"
// @Success 200 {object} common.V2Response
// @Router /api/v2/saas/communities/{id}/subscription [get]
func (h *ProvisioningHandler) GetSubscription(c *gin.Context) {
	siteID := c.Param("id")
	sub, err := h.provisioningSvc.GetSubscription(c.Request.Context(), siteID)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusNotFound, err.Error(), nil)
		return
	}
	common.V2Success(c, sub)
}

// ChangePlan godoc
// @Summary 플랜 변경 (업/다운그레이드)
// @Tags saas
// @Accept json
// @Param id path string true "사이트 ID"
// @Param body body domain.ChangePlanRequest true "플랜 변경 정보"
// @Success 200 {object} common.V2Response
// @Router /api/v2/saas/communities/{id}/subscription/plan [put]
func (h *ProvisioningHandler) ChangePlan(c *gin.Context) {
	siteID := c.Param("id")
	var req domain.ChangePlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다", err)
		return
	}

	if err := h.provisioningSvc.ChangePlan(c.Request.Context(), siteID, &req); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	common.V2Success(c, gin.H{"message": "플랜이 변경되었습니다", "plan": req.Plan})
}

// CancelSubscription godoc
// @Summary 구독 취소
// @Tags saas
// @Param id path string true "사이트 ID"
// @Success 200 {object} common.V2Response
// @Router /api/v2/saas/communities/{id}/subscription/cancel [post]
func (h *ProvisioningHandler) CancelSubscription(c *gin.Context) {
	siteID := c.Param("id")
	if err := h.provisioningSvc.CancelSubscription(c.Request.Context(), siteID); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	common.V2Success(c, gin.H{"message": "구독이 취소되었습니다. 현재 결제 기간 종료 시 비활성화됩니다"})
}

// GetInvoices godoc
// @Summary 청구서 목록 조회
// @Tags saas
// @Param id path string true "사이트 ID"
// @Param page query int false "페이지" default(1)
// @Param per_page query int false "페이지당 항목" default(20)
// @Success 200 {object} common.V2Response
// @Router /api/v2/saas/communities/{id}/invoices [get]
func (h *ProvisioningHandler) GetInvoices(c *gin.Context) {
	siteID := c.Param("id")
	page := parseIntQuery(c, "page", 1)
	perPage := parseIntQuery(c, "per_page", 20)

	invoices, total, err := h.provisioningSvc.GetInvoices(c.Request.Context(), siteID, page, perPage)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "청구서 조회 실패", err)
		return
	}
	common.V2SuccessWithMeta(c, invoices, common.NewV2Meta(page, perPage, total))
}

// GetPricing godoc
// @Summary 플랜 가격 정보 조회
// @Tags saas
// @Success 200 {object} common.V2Response
// @Router /api/v2/saas/pricing [get]
func (h *ProvisioningHandler) GetPricing(c *gin.Context) {
	pricing := h.provisioningSvc.GetPricing()
	common.V2Success(c, pricing)
}
