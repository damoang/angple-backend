package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/internal/plugins/commerce/domain"
	"github.com/damoang/angple-backend/internal/plugins/commerce/service"
	"github.com/gin-gonic/gin"
)

// SettlementHandler 정산 HTTP 핸들러
type SettlementHandler struct {
	service service.SettlementService
}

// NewSettlementHandler 생성자
func NewSettlementHandler(svc service.SettlementService) *SettlementHandler {
	return &SettlementHandler{service: svc}
}

// ListSettlements godoc
// @Summary      정산 목록 조회
// @Description  내 정산 내역을 조회합니다
// @Tags         commerce-settlements
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        page       query     int     false  "페이지 (기본: 1)"
// @Param        limit      query     int     false  "페이지당 개수 (기본: 20, 최대: 100)"
// @Param        status     query     string  false  "상태 필터 (pending, processing, completed, failed)"
// @Param        year       query     int     false  "연도 필터"
// @Param        month      query     int     false  "월 필터"
// @Param        sort_by    query     string  false  "정렬 기준 (created_at, period_start, settlement_amount)"
// @Param        sort_order query     string  false  "정렬 순서 (asc, desc)"
// @Success      200  {object}  common.APIResponse{data=[]domain.SettlementResponse}
// @Failure      401  {object}  common.APIResponse
// @Router       /plugins/commerce/settlements [get]
func (h *SettlementHandler) ListSettlements(c *gin.Context) {
	sellerID, err := h.getSellerID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	var req domain.SettlementListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request parameters", err)
		return
	}

	settlements, total, err := h.service.ListSettlements(sellerID, &req)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch settlements", err)
		return
	}

	page := req.Page
	if page < 1 {
		page = 1
	}
	limit := req.Limit
	if limit < 1 {
		limit = 20
	}

	meta := &common.Meta{
		Page:  page,
		Limit: limit,
		Total: total,
	}

	common.SuccessResponse(c, settlements, meta)
}

// GetSettlement godoc
// @Summary      정산 상세 조회
// @Description  정산 상세 정보를 조회합니다
// @Tags         commerce-settlements
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "정산 ID"
// @Success      200  {object}  common.APIResponse{data=domain.SettlementResponse}
// @Failure      401  {object}  common.APIResponse
// @Failure      403  {object}  common.APIResponse
// @Failure      404  {object}  common.APIResponse
// @Router       /plugins/commerce/settlements/{id} [get]
func (h *SettlementHandler) GetSettlement(c *gin.Context) {
	sellerID, err := h.getSellerID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid settlement ID", err)
		return
	}

	settlement, err := h.service.GetSettlement(id, sellerID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrSettlementNotFound):
			common.ErrorResponse(c, http.StatusNotFound, "Settlement not found", err)
		case errors.Is(err, service.ErrSettlementForbidden):
			common.ErrorResponse(c, http.StatusForbidden, "Forbidden", err)
		default:
			common.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch settlement", err)
		}
		return
	}

	common.SuccessResponse(c, settlement, nil)
}

// GetSummary godoc
// @Summary      정산 요약 조회
// @Description  내 정산 요약 정보를 조회합니다
// @Tags         commerce-settlements
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  common.APIResponse{data=domain.SettlementSummary}
// @Failure      401  {object}  common.APIResponse
// @Router       /plugins/commerce/settlements/summary [get]
func (h *SettlementHandler) GetSummary(c *gin.Context) {
	sellerID, err := h.getSellerID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	summary, err := h.service.GetSettlementSummary(sellerID)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch settlement summary", err)
		return
	}

	common.SuccessResponse(c, summary, nil)
}

// CreateSettlementRequest 정산 생성 요청
type CreateSettlementRequest struct {
	PeriodStart string `json:"period_start" binding:"required"` // YYYY-MM-DD
	PeriodEnd   string `json:"period_end" binding:"required"`   // YYYY-MM-DD
}

// CreateSettlement godoc
// @Summary      정산 생성 (관리자)
// @Description  특정 기간의 정산을 생성합니다
// @Tags         commerce-settlements
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        seller_id  path      int                       true  "판매자 ID"
// @Param        request    body      CreateSettlementRequest   true  "정산 생성 요청"
// @Success      201  {object}  common.APIResponse{data=domain.SettlementResponse}
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      409  {object}  common.APIResponse
// @Router       /plugins/commerce/admin/settlements/{seller_id} [post]
func (h *SettlementHandler) CreateSettlement(c *gin.Context) {
	sellerID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid seller ID", err)
		return
	}

	var req CreateSettlementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	// 날짜 파싱
	periodStart, err := time.Parse("2006-01-02", req.PeriodStart)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid period_start format (use YYYY-MM-DD)", err)
		return
	}

	periodEnd, err := time.Parse("2006-01-02", req.PeriodEnd)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid period_end format (use YYYY-MM-DD)", err)
		return
	}

	// 기간 끝 날짜는 다음날 00:00으로 설정 (해당 날짜 포함)
	periodEnd = periodEnd.Add(24 * time.Hour)

	settlement, err := h.service.CreateSettlement(sellerID, periodStart, periodEnd)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidPeriod):
			common.ErrorResponse(c, http.StatusBadRequest, "Invalid period", err)
		case errors.Is(err, service.ErrDuplicateSettlement):
			common.ErrorResponse(c, http.StatusConflict, "Settlement already exists for this period", err)
		case errors.Is(err, service.ErrSettlementNoOrders):
			common.ErrorResponse(c, http.StatusBadRequest, "No orders to settle for this period", err)
		default:
			common.ErrorResponse(c, http.StatusInternalServerError, "Failed to create settlement", err)
		}
		return
	}

	c.JSON(http.StatusCreated, common.APIResponse{Data: settlement})
}

// ProcessSettlement godoc
// @Summary      정산 처리 (관리자)
// @Description  정산을 처리합니다 (송금)
// @Tags         commerce-settlements
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path      int                            true  "정산 ID"
// @Param        request  body      domain.ProcessSettlementRequest true  "정산 처리 요청"
// @Success      200  {object}  common.APIResponse
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      404  {object}  common.APIResponse
// @Router       /plugins/commerce/admin/settlements/{id}/process [post]
func (h *SettlementHandler) ProcessSettlement(c *gin.Context) {
	adminID, err := h.getAdminID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid settlement ID", err)
		return
	}

	var req domain.ProcessSettlementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	if err := h.service.ProcessSettlement(id, adminID, &req); err != nil {
		switch {
		case errors.Is(err, service.ErrSettlementNotFound):
			common.ErrorResponse(c, http.StatusNotFound, "Settlement not found", err)
		case errors.Is(err, service.ErrSettlementNotPending):
			common.ErrorResponse(c, http.StatusBadRequest, "Settlement is not in pending status", err)
		default:
			common.ErrorResponse(c, http.StatusInternalServerError, "Failed to process settlement", err)
		}
		return
	}

	common.SuccessResponse(c, gin.H{"message": "Settlement processed successfully"}, nil)
}

// ListAllSettlements godoc
// @Summary      전체 정산 목록 조회 (관리자)
// @Description  모든 판매자의 정산 내역을 조회합니다
// @Tags         commerce-settlements
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        page       query     int     false  "페이지 (기본: 1)"
// @Param        limit      query     int     false  "페이지당 개수 (기본: 20, 최대: 100)"
// @Param        status     query     string  false  "상태 필터 (pending, processing, completed, failed)"
// @Param        year       query     int     false  "연도 필터"
// @Param        month      query     int     false  "월 필터"
// @Param        sort_by    query     string  false  "정렬 기준 (created_at, period_start, settlement_amount)"
// @Param        sort_order query     string  false  "정렬 순서 (asc, desc)"
// @Success      200  {object}  common.APIResponse{data=[]domain.SettlementResponse}
// @Failure      401  {object}  common.APIResponse
// @Router       /plugins/commerce/admin/settlements [get]
func (h *SettlementHandler) ListAllSettlements(c *gin.Context) {
	var req domain.SettlementListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request parameters", err)
		return
	}

	// sellerID = 0 은 전체 조회
	settlements, total, err := h.service.ListSettlements(0, &req)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch settlements", err)
		return
	}

	page := req.Page
	if page < 1 {
		page = 1
	}
	limit := req.Limit
	if limit < 1 {
		limit = 20
	}

	meta := &common.Meta{
		Page:  page,
		Limit: limit,
		Total: total,
	}

	common.SuccessResponse(c, settlements, meta)
}

// getSellerID JWT에서 판매자 ID 추출
func (h *SettlementHandler) getSellerID(c *gin.Context) (uint64, error) {
	userIDStr := middleware.GetUserID(c)
	if userIDStr == "" {
		return 0, errors.New("user not authenticated")
	}

	id, err := strconv.ParseUint(userIDStr, 10, 64)
	if err != nil {
		return 0, errors.New("invalid user ID format")
	}
	return id, nil
}

// getAdminID JWT에서 관리자 ID 추출
func (h *SettlementHandler) getAdminID(c *gin.Context) (uint64, error) {
	// TODO: 관리자 권한 확인 로직 추가
	userIDStr := middleware.GetUserID(c)
	if userIDStr == "" {
		return 0, errors.New("user not authenticated")
	}

	id, err := strconv.ParseUint(userIDStr, 10, 64)
	if err != nil {
		return 0, errors.New("invalid user ID format")
	}
	return id, nil
}
