package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/internal/plugins/commerce/domain"
	"github.com/damoang/angple-backend/internal/plugins/commerce/service"
	"github.com/gin-gonic/gin"
)

// CouponHandler 쿠폰 HTTP 핸들러
type CouponHandler struct {
	service service.CouponService
}

// NewCouponHandler 생성자
func NewCouponHandler(svc service.CouponService) *CouponHandler {
	return &CouponHandler{service: svc}
}

// CreateCoupon godoc
// @Summary      쿠폰 생성
// @Description  새로운 쿠폰을 생성합니다 (관리자 전용)
// @Tags         commerce-coupons
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request  body      domain.CreateCouponRequest  true  "쿠폰 생성 요청"
// @Success      201  {object}  common.APIResponse{data=domain.CouponResponse}
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      409  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/commerce/admin/coupons [post]
func (h *CouponHandler) CreateCoupon(c *gin.Context) {
	adminID, err := h.getUserID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	var req domain.CreateCouponRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	coupon, err := h.service.CreateCoupon(adminID, &req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrCouponCodeExists):
			common.ErrorResponse(c, http.StatusConflict, "Coupon code already exists", err)
		case errors.Is(err, service.ErrInvalidDiscountType):
			common.ErrorResponse(c, http.StatusBadRequest, "Invalid discount type", err)
		default:
			common.ErrorResponse(c, http.StatusInternalServerError, "Failed to create coupon", err)
		}
		return
	}

	c.JSON(http.StatusCreated, common.APIResponse{Data: coupon})
}

// UpdateCoupon godoc
// @Summary      쿠폰 수정
// @Description  기존 쿠폰을 수정합니다 (관리자 전용)
// @Tags         commerce-coupons
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path      int                         true  "쿠폰 ID"
// @Param        request  body      domain.UpdateCouponRequest  true  "쿠폰 수정 요청"
// @Success      200  {object}  common.APIResponse{data=domain.CouponResponse}
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      404  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/commerce/admin/coupons/{id} [put]
func (h *CouponHandler) UpdateCoupon(c *gin.Context) {
	_, err := h.getUserID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	couponID, err := h.getCouponID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid coupon ID", err)
		return
	}

	var req domain.UpdateCouponRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	coupon, err := h.service.UpdateCoupon(couponID, &req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrCouponNotFound):
			common.ErrorResponse(c, http.StatusNotFound, "Coupon not found", err)
		default:
			common.ErrorResponse(c, http.StatusInternalServerError, "Failed to update coupon", err)
		}
		return
	}

	common.SuccessResponse(c, coupon, nil)
}

// DeleteCoupon godoc
// @Summary      쿠폰 삭제
// @Description  쿠폰을 삭제합니다 (관리자 전용)
// @Tags         commerce-coupons
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "쿠폰 ID"
// @Success      204  "No Content"
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      404  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/commerce/admin/coupons/{id} [delete]
func (h *CouponHandler) DeleteCoupon(c *gin.Context) {
	_, err := h.getUserID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	couponID, err := h.getCouponID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid coupon ID", err)
		return
	}

	err = h.service.DeleteCoupon(couponID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrCouponNotFound):
			common.ErrorResponse(c, http.StatusNotFound, "Coupon not found", err)
		default:
			common.ErrorResponse(c, http.StatusInternalServerError, "Failed to delete coupon", err)
		}
		return
	}

	c.Status(http.StatusNoContent)
}

// GetCoupon godoc
// @Summary      쿠폰 조회
// @Description  쿠폰 상세 정보를 조회합니다 (관리자 전용)
// @Tags         commerce-coupons
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "쿠폰 ID"
// @Success      200  {object}  common.APIResponse{data=domain.CouponResponse}
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      404  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/commerce/admin/coupons/{id} [get]
func (h *CouponHandler) GetCoupon(c *gin.Context) {
	_, err := h.getUserID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	couponID, err := h.getCouponID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid coupon ID", err)
		return
	}

	coupon, err := h.service.GetCoupon(couponID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrCouponNotFound):
			common.ErrorResponse(c, http.StatusNotFound, "Coupon not found", err)
		default:
			common.ErrorResponse(c, http.StatusInternalServerError, "Failed to get coupon", err)
		}
		return
	}

	common.SuccessResponse(c, coupon, nil)
}

// ListCoupons godoc
// @Summary      쿠폰 목록 조회
// @Description  쿠폰 목록을 조회합니다 (관리자 전용)
// @Tags         commerce-coupons
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        page      query     int     false  "페이지 번호"
// @Param        limit     query     int     false  "페이지당 항목 수"
// @Param        status    query     string  false  "상태 필터 (active, inactive, expired)"
// @Param        is_public query     bool    false  "공개 여부 필터"
// @Success      200  {object}  common.APIResponse{data=[]domain.CouponResponse}
// @Failure      401  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/commerce/admin/coupons [get]
func (h *CouponHandler) ListCoupons(c *gin.Context) {
	_, err := h.getUserID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	var req domain.CouponListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid query parameters", err)
		return
	}

	coupons, total, err := h.service.ListCoupons(&req)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to list coupons", err)
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

	common.SuccessResponse(c, coupons, meta)
}

// ValidateCoupon godoc
// @Summary      쿠폰 유효성 검증
// @Description  쿠폰 코드의 유효성을 검증합니다
// @Tags         commerce-coupons
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request  body      domain.ValidateCouponRequest  true  "쿠폰 검증 요청"
// @Param        order_amount query  float64  false  "주문 금액"
// @Success      200  {object}  common.APIResponse{data=domain.ValidateCouponResponse}
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/commerce/coupons/validate [post]
func (h *CouponHandler) ValidateCoupon(c *gin.Context) {
	userID, err := h.getUserID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	var req domain.ValidateCouponRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	orderAmount, _ := strconv.ParseFloat(c.Query("order_amount"), 64)
	if orderAmount == 0 {
		orderAmount = 1 // 기본값: 최소 금액 검사를 위해
	}

	result, err := h.service.ValidateCoupon(userID, req.Code, orderAmount)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to validate coupon", err)
		return
	}

	common.SuccessResponse(c, result, nil)
}

// GetPublicCoupons godoc
// @Summary      공개 쿠폰 목록 조회
// @Description  공개된 사용 가능한 쿠폰 목록을 조회합니다
// @Tags         commerce-coupons
// @Accept       json
// @Produce      json
// @Success      200  {object}  common.APIResponse{data=[]domain.CouponResponse}
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/commerce/coupons/public [get]
func (h *CouponHandler) GetPublicCoupons(c *gin.Context) {
	coupons, err := h.service.GetPublicCoupons()
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to get public coupons", err)
		return
	}

	common.SuccessResponse(c, coupons, nil)
}

// ApplyCoupon godoc
// @Summary      주문에 쿠폰 적용
// @Description  주문에 쿠폰을 적용합니다
// @Tags         commerce-coupons
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request  body      domain.ApplyCouponRequest  true  "쿠폰 적용 요청"
// @Success      200  {object}  common.APIResponse{data=map[string]interface{}}
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      404  {object}  common.APIResponse
// @Failure      409  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/commerce/coupons/apply [post]
func (h *CouponHandler) ApplyCoupon(c *gin.Context) {
	userID, err := h.getUserID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	var req domain.ApplyCouponRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	discountAmount, err := h.service.ApplyCoupon(userID, req.OrderID, req.Code)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrOrderNotFound):
			common.ErrorResponse(c, http.StatusNotFound, "Order not found", err)
		case errors.Is(err, service.ErrOrderForbidden):
			common.ErrorResponse(c, http.StatusForbidden, "Forbidden", err)
		case errors.Is(err, service.ErrCouponAlreadyApplied):
			common.ErrorResponse(c, http.StatusConflict, "Coupon already applied", err)
		case errors.Is(err, service.ErrCouponNotApplicable):
			common.ErrorResponse(c, http.StatusBadRequest, "Coupon not applicable", err)
		default:
			common.ErrorResponse(c, http.StatusBadRequest, err.Error(), err)
		}
		return
	}

	common.SuccessResponse(c, map[string]interface{}{
		"discount_amount": discountAmount,
		"message":         "쿠폰이 성공적으로 적용되었습니다",
	}, nil)
}

// RemoveCoupon godoc
// @Summary      주문에서 쿠폰 제거
// @Description  주문에서 적용된 쿠폰을 제거합니다
// @Tags         commerce-coupons
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        order_id  path      int  true  "주문 ID"
// @Success      200  {object}  common.APIResponse
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      404  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/commerce/orders/{order_id}/coupon [delete]
func (h *CouponHandler) RemoveCoupon(c *gin.Context) {
	_, err := h.getUserID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	orderID, err := strconv.ParseUint(c.Param("order_id"), 10, 64)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid order ID", err)
		return
	}

	err = h.service.RemoveCoupon(orderID)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to remove coupon", err)
		return
	}

	common.SuccessResponse(c, map[string]string{
		"message": "쿠폰이 성공적으로 제거되었습니다",
	}, nil)
}

// getUserID JWT에서 사용자 ID 추출
func (h *CouponHandler) getUserID(c *gin.Context) (uint64, error) {
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

// getCouponID 경로에서 쿠폰 ID 추출
func (h *CouponHandler) getCouponID(c *gin.Context) (uint64, error) {
	idStr := c.Param("id")
	return strconv.ParseUint(idStr, 10, 64)
}
