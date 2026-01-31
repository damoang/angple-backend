package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/plugins/commerce/carrier"
	"github.com/damoang/angple-backend/internal/plugins/commerce/domain"
	"github.com/damoang/angple-backend/internal/plugins/commerce/service"
	"github.com/gin-gonic/gin"
)

// ShippingHandler 배송 핸들러
type ShippingHandler struct {
	service service.ShippingService
}

// NewShippingHandler 생성자
func NewShippingHandler(service service.ShippingService) *ShippingHandler {
	return &ShippingHandler{service: service}
}

// GetCarriers 배송사 목록 조회
// @Summary 배송사 목록 조회
// @Tags Shipping
// @Produce json
// @Success 200 {object} domain.ShippingCarrierListResponse
// @Router /shipping/carriers [get]
func (h *ShippingHandler) GetCarriers(c *gin.Context) {
	carriers := h.service.GetCarriers()
	common.SuccessResponse(c, carriers, nil)
}

// RegisterShipping 송장번호 등록 (판매자용)
// @Summary 송장번호 등록
// @Tags Shipping
// @Accept json
// @Produce json
// @Param order_id path int true "주문 ID"
// @Param request body domain.RegisterShippingRequest true "송장 정보"
// @Success 200 {object} common.Response
// @Failure 400 {object} common.Response
// @Failure 401 {object} common.Response
// @Failure 403 {object} common.Response
// @Failure 404 {object} common.Response
// @Router /seller/orders/{order_id}/shipping [post]
func (h *ShippingHandler) RegisterShipping(c *gin.Context) {
	// 판매자 ID 가져오기
	sellerID, exists := c.Get("user_id")
	if !exists {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", errors.New("unauthorized"))
		return
	}

	// 주문 ID 파싱
	orderID, err := strconv.ParseUint(c.Param("order_id"), 10, 64)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "올바르지 않은 주문 ID입니다", err)
		return
	}

	// 요청 바디 파싱
	var req domain.RegisterShippingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "잘못된 요청입니다", err)
		return
	}

	// 송장번호 등록
	if err := h.service.RegisterShipping(c.Request.Context(), sellerID.(uint64), orderID, &req); err != nil {
		switch err {
		case service.ErrOrderNotFound:
			common.ErrorResponse(c, http.StatusNotFound, "주문을 찾을 수 없습니다", err)
		case service.ErrOrderForbidden:
			common.ErrorResponse(c, http.StatusForbidden, "해당 주문에 대한 권한이 없습니다", err)
		case service.ErrShippingNotAllowed:
			common.ErrorResponse(c, http.StatusBadRequest, "디지털 상품은 배송 정보를 등록할 수 없습니다", err)
		case service.ErrShippingAlreadySet:
			common.ErrorResponse(c, http.StatusConflict, "이미 송장번호가 등록되어 있습니다", err)
		case service.ErrInvalidShippingStatus:
			common.ErrorResponse(c, http.StatusBadRequest, "현재 주문 상태에서는 송장번호를 등록할 수 없습니다", err)
		case service.ErrCarrierNotSupported:
			common.ErrorResponse(c, http.StatusBadRequest, "지원하지 않는 배송사입니다", err)
		case carrier.ErrInvalidTrackingNo:
			common.ErrorResponse(c, http.StatusBadRequest, "올바르지 않은 송장번호 형식입니다", err)
		default:
			common.ErrorResponse(c, http.StatusInternalServerError, "서버 오류가 발생했습니다", err)
		}
		return
	}

	common.SuccessResponse(c, map[string]string{"message": "송장번호가 등록되었습니다"}, nil)
}

// TrackShipping 배송 추적 (구매자용)
// @Summary 배송 추적
// @Tags Shipping
// @Produce json
// @Param order_id path int true "주문 ID"
// @Success 200 {object} domain.TrackingResponse
// @Failure 400 {object} common.Response
// @Failure 401 {object} common.Response
// @Failure 403 {object} common.Response
// @Failure 404 {object} common.Response
// @Router /orders/{id}/tracking [get]
func (h *ShippingHandler) TrackShipping(c *gin.Context) {
	// 사용자 ID 가져오기
	userID, exists := c.Get("user_id")
	if !exists {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", errors.New("unauthorized"))
		return
	}

	// 주문 ID 파싱
	orderID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "올바르지 않은 주문 ID입니다", err)
		return
	}

	// 배송 추적
	trackingResp, err := h.service.TrackShipping(c.Request.Context(), userID.(uint64), orderID)
	if err != nil {
		switch err {
		case service.ErrOrderNotFound:
			common.ErrorResponse(c, http.StatusNotFound, "주문을 찾을 수 없습니다", err)
		case service.ErrOrderForbidden:
			common.ErrorResponse(c, http.StatusForbidden, "해당 주문에 대한 권한이 없습니다", err)
		default:
			if err.Error() == "tracking number not set" {
				common.ErrorResponse(c, http.StatusNotFound, "아직 송장번호가 등록되지 않았습니다", err)
			} else {
				common.ErrorResponse(c, http.StatusInternalServerError, "서버 오류가 발생했습니다", err)
			}
		}
		return
	}

	common.SuccessResponse(c, trackingResp, nil)
}

// MarkDelivered 배송 완료 처리 (판매자용)
// @Summary 배송 완료 처리
// @Tags Shipping
// @Produce json
// @Param order_id path int true "주문 ID"
// @Success 200 {object} common.Response
// @Failure 400 {object} common.Response
// @Failure 401 {object} common.Response
// @Failure 403 {object} common.Response
// @Failure 404 {object} common.Response
// @Router /seller/orders/{order_id}/delivered [post]
func (h *ShippingHandler) MarkDelivered(c *gin.Context) {
	// 판매자 ID 가져오기
	sellerID, exists := c.Get("user_id")
	if !exists {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", errors.New("unauthorized"))
		return
	}

	// 주문 ID 파싱
	orderID, err := strconv.ParseUint(c.Param("order_id"), 10, 64)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "올바르지 않은 주문 ID입니다", err)
		return
	}

	// 배송 완료 처리
	if err := h.service.MarkDelivered(c.Request.Context(), sellerID.(uint64), orderID); err != nil {
		switch err {
		case service.ErrOrderNotFound:
			common.ErrorResponse(c, http.StatusNotFound, "주문을 찾을 수 없습니다", err)
		case service.ErrOrderForbidden:
			common.ErrorResponse(c, http.StatusForbidden, "해당 주문에 대한 권한이 없습니다", err)
		case service.ErrInvalidShippingStatus:
			common.ErrorResponse(c, http.StatusBadRequest, "배송 중 상태의 주문만 배송 완료 처리할 수 있습니다", err)
		default:
			common.ErrorResponse(c, http.StatusInternalServerError, "서버 오류가 발생했습니다", err)
		}
		return
	}

	common.SuccessResponse(c, map[string]string{"message": "배송 완료 처리되었습니다"}, nil)
}
