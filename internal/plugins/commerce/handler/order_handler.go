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

// OrderHandler 주문 HTTP 핸들러
type OrderHandler struct {
	service service.OrderService
}

// NewOrderHandler 생성자
func NewOrderHandler(svc service.OrderService) *OrderHandler {
	return &OrderHandler{service: svc}
}

// CreateOrder godoc
// @Summary      주문 생성
// @Description  장바구니에 담긴 상품으로 주문을 생성합니다
// @Tags         commerce-orders
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request  body      domain.CreateOrderRequest  true  "주문 생성 요청"
// @Success      201  {object}  common.APIResponse{data=domain.OrderResponse}
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      409  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/commerce/orders [post]
func (h *OrderHandler) CreateOrder(c *gin.Context) {
	userID, err := h.getUserID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	var req domain.CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	// 클라이언트 정보
	ipAddress := c.ClientIP()
	userAgent := c.GetHeader("User-Agent")

	order, err := h.service.CreateOrder(userID, &req, ipAddress, userAgent)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrEmptyCart):
			common.ErrorResponse(c, http.StatusBadRequest, "Cart is empty", err)
		case errors.Is(err, service.ErrShippingInfoRequired):
			common.ErrorResponse(c, http.StatusBadRequest, "Shipping info is required for physical products", err)
		case errors.Is(err, service.ErrInsufficientStock):
			common.ErrorResponse(c, http.StatusConflict, "Insufficient stock", err)
		case errors.Is(err, service.ErrProductNotAvailable):
			common.ErrorResponse(c, http.StatusBadRequest, "Some products are not available", err)
		default:
			common.ErrorResponse(c, http.StatusInternalServerError, "Failed to create order", err)
		}
		return
	}

	c.JSON(http.StatusCreated, common.APIResponse{Data: order})
}

// ListOrders godoc
// @Summary      내 주문 목록 조회
// @Description  사용자의 주문 목록을 조회합니다
// @Tags         commerce-orders
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        page       query     int     false  "페이지 번호 (기본값: 1)"
// @Param        limit      query     int     false  "페이지당 항목 수 (기본값: 20, 최대: 100)"
// @Param        status     query     string  false  "주문 상태 (pending, paid, processing, shipped, delivered, completed, cancelled, refunded)"
// @Param        sort_by    query     string  false  "정렬 기준 (created_at, total)"
// @Param        sort_order query     string  false  "정렬 순서 (asc, desc)"
// @Success      200  {object}  common.APIResponse{data=[]domain.OrderResponse}
// @Failure      401  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/commerce/orders [get]
func (h *OrderHandler) ListOrders(c *gin.Context) {
	userID, err := h.getUserID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	var req domain.OrderListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request parameters", err)
		return
	}

	orders, meta, err := h.service.ListOrders(userID, &req)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch orders", err)
		return
	}

	common.SuccessResponse(c, orders, meta)
}

// GetOrder godoc
// @Summary      주문 상세 조회
// @Description  주문의 상세 정보를 조회합니다
// @Tags         commerce-orders
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "주문 ID"
// @Success      200  {object}  common.APIResponse{data=domain.OrderResponse}
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      403  {object}  common.APIResponse
// @Failure      404  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/commerce/orders/{id} [get]
func (h *OrderHandler) GetOrder(c *gin.Context) {
	userID, err := h.getUserID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	orderID, err := h.getOrderID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid order ID", err)
		return
	}

	order, err := h.service.GetOrder(userID, orderID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrOrderNotFound):
			common.ErrorResponse(c, http.StatusNotFound, "Order not found", err)
		case errors.Is(err, service.ErrOrderForbidden):
			common.ErrorResponse(c, http.StatusForbidden, "Forbidden", err)
		default:
			common.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch order", err)
		}
		return
	}

	common.SuccessResponse(c, order, nil)
}

// CancelOrder godoc
// @Summary      주문 취소
// @Description  주문을 취소합니다 (pending 상태만 가능)
// @Tags         commerce-orders
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path      int                        true   "주문 ID"
// @Param        request  body      domain.CancelOrderRequest  false  "취소 요청"
// @Success      204  "No Content"
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      403  {object}  common.APIResponse
// @Failure      404  {object}  common.APIResponse
// @Failure      409  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/commerce/orders/{id}/cancel [post]
func (h *OrderHandler) CancelOrder(c *gin.Context) {
	userID, err := h.getUserID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	orderID, err := h.getOrderID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid order ID", err)
		return
	}

	var req domain.CancelOrderRequest
	// 취소 사유는 선택 사항
	_ = c.ShouldBindJSON(&req)

	err = h.service.CancelOrder(userID, orderID, &req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrOrderNotFound):
			common.ErrorResponse(c, http.StatusNotFound, "Order not found", err)
		case errors.Is(err, service.ErrOrderForbidden):
			common.ErrorResponse(c, http.StatusForbidden, "Forbidden", err)
		case errors.Is(err, service.ErrOrderCannotBeCancelled):
			common.ErrorResponse(c, http.StatusConflict, "Order cannot be cancelled", err)
		default:
			common.ErrorResponse(c, http.StatusInternalServerError, "Failed to cancel order", err)
		}
		return
	}

	c.Status(http.StatusNoContent)
}

// getUserID JWT에서 사용자 ID 추출
func (h *OrderHandler) getUserID(c *gin.Context) (uint64, error) {
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

// getOrderID 경로에서 주문 ID 추출
func (h *OrderHandler) getOrderID(c *gin.Context) (uint64, error) {
	idStr := c.Param("id")
	return strconv.ParseUint(idStr, 10, 64)
}
