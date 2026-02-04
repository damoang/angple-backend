package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/internal/plugins/commerce/domain"
	"github.com/damoang/angple-backend/internal/plugins/commerce/service"
	"github.com/gin-gonic/gin"
)

// PaymentHandler 결제 HTTP 핸들러
type PaymentHandler struct {
	service service.PaymentService
}

// NewPaymentHandler 생성자
func NewPaymentHandler(svc service.PaymentService) *PaymentHandler {
	return &PaymentHandler{service: svc}
}

// PreparePayment godoc
// @Summary      결제 준비
// @Description  결제를 준비하고 PG 결제창 호출에 필요한 정보를 반환합니다
// @Tags         commerce-payments
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request  body      domain.PreparePaymentRequest  true  "결제 준비 요청"
// @Success      200  {object}  common.APIResponse{data=domain.PreparePaymentResponse}
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      404  {object}  common.APIResponse
// @Failure      409  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/commerce/payments/prepare [post]
func (h *PaymentHandler) PreparePayment(c *gin.Context) {
	userID, err := h.getUserID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	var req domain.PreparePaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	ctx := context.Background()
	payment, err := h.service.PreparePayment(ctx, userID, &req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrOrderNotFound):
			common.ErrorResponse(c, http.StatusNotFound, "Order not found", err)
		case errors.Is(err, service.ErrOrderForbidden):
			common.ErrorResponse(c, http.StatusForbidden, "Forbidden", err)
		case errors.Is(err, service.ErrPaymentAlreadyPaid):
			common.ErrorResponse(c, http.StatusConflict, "Payment already completed", err)
		case errors.Is(err, service.ErrPGNotSupported):
			common.ErrorResponse(c, http.StatusBadRequest, "Payment gateway not supported", err)
		default:
			common.ErrorResponse(c, http.StatusInternalServerError, "Failed to prepare payment", err)
		}
		return
	}

	common.SuccessResponse(c, payment, nil)
}

// CompletePayment godoc
// @Summary      결제 완료
// @Description  PG 결제창에서 결제 완료 후 호출하여 결제를 확정합니다
// @Tags         commerce-payments
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request  body      domain.CompletePaymentRequest  true  "결제 완료 요청"
// @Success      200  {object}  common.APIResponse{data=domain.PaymentResponse}
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      404  {object}  common.APIResponse
// @Failure      409  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/commerce/payments/complete [post]
func (h *PaymentHandler) CompletePayment(c *gin.Context) {
	userID, err := h.getUserID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	var req domain.CompletePaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	ctx := context.Background()
	payment, err := h.service.CompletePayment(ctx, userID, &req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrPaymentNotFound):
			common.ErrorResponse(c, http.StatusNotFound, "Payment not found", err)
		case errors.Is(err, service.ErrPaymentForbidden):
			common.ErrorResponse(c, http.StatusForbidden, "Forbidden", err)
		case errors.Is(err, service.ErrPaymentAlreadyPaid):
			common.ErrorResponse(c, http.StatusConflict, "Payment already completed", err)
		case errors.Is(err, service.ErrAmountMismatch):
			common.ErrorResponse(c, http.StatusBadRequest, "Amount mismatch", err)
		case errors.Is(err, service.ErrPGNotSupported):
			common.ErrorResponse(c, http.StatusBadRequest, "Payment gateway not supported", err)
		default:
			common.ErrorResponse(c, http.StatusInternalServerError, "Failed to complete payment", err)
		}
		return
	}

	common.SuccessResponse(c, payment, nil)
}

// CancelPayment godoc
// @Summary      결제 취소
// @Description  결제를 취소합니다 (전체 취소 또는 부분 취소)
// @Tags         commerce-payments
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request  body      domain.CancelPaymentRequest  true  "결제 취소 요청"
// @Success      204  "No Content"
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      404  {object}  common.APIResponse
// @Failure      409  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/commerce/payments/cancel [post]
func (h *PaymentHandler) CancelPayment(c *gin.Context) {
	userID, err := h.getUserID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	var req domain.CancelPaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	ctx := context.Background()
	err = h.service.CancelPayment(ctx, userID, &req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrPaymentNotFound):
			common.ErrorResponse(c, http.StatusNotFound, "Payment not found", err)
		case errors.Is(err, service.ErrPaymentForbidden):
			common.ErrorResponse(c, http.StatusForbidden, "Forbidden", err)
		case errors.Is(err, service.ErrCancelNotAllowed):
			common.ErrorResponse(c, http.StatusConflict, "Payment cancel not allowed", err)
		case errors.Is(err, service.ErrInvalidAmount):
			common.ErrorResponse(c, http.StatusBadRequest, "Invalid cancel amount", err)
		default:
			common.ErrorResponse(c, http.StatusInternalServerError, "Failed to cancel payment", err)
		}
		return
	}

	c.Status(http.StatusNoContent)
}

// GetPayment godoc
// @Summary      결제 조회
// @Description  결제 상세 정보를 조회합니다
// @Tags         commerce-payments
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "결제 ID"
// @Success      200  {object}  common.APIResponse{data=domain.PaymentResponse}
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      403  {object}  common.APIResponse
// @Failure      404  {object}  common.APIResponse
// @Router       /plugins/commerce/payments/{id} [get]
func (h *PaymentHandler) GetPayment(c *gin.Context) {
	userID, err := h.getUserID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	paymentID, err := h.getPaymentID(c)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid payment ID", err)
		return
	}

	ctx := context.Background()
	payment, err := h.service.GetPayment(ctx, userID, paymentID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrPaymentNotFound):
			common.ErrorResponse(c, http.StatusNotFound, "Payment not found", err)
		case errors.Is(err, service.ErrPaymentForbidden):
			common.ErrorResponse(c, http.StatusForbidden, "Forbidden", err)
		default:
			common.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch payment", err)
		}
		return
	}

	common.SuccessResponse(c, payment, nil)
}

// HandleWebhook godoc
// @Summary      PG 웹훅 처리
// @Description  PG사에서 전송하는 웹훅을 처리합니다
// @Tags         commerce-webhooks
// @Accept       json
// @Produce      json
// @Param        provider  path      string  true  "PG사 (inicis, tosspayments)"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /plugins/commerce/webhooks/{provider} [post]
func (h *PaymentHandler) HandleWebhook(c *gin.Context) {
	provider := c.Param("provider")
	if provider == "" {
		common.ErrorResponse(c, http.StatusBadRequest, "Provider is required", nil)
		return
	}

	// 요청 본문 읽기
	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Failed to read request body", err)
		return
	}

	ctx := context.Background()
	pgProvider := domain.PGProvider(provider)

	err = h.service.HandleWebhook(ctx, pgProvider, payload)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrPGNotSupported):
			common.ErrorResponse(c, http.StatusBadRequest, "Payment gateway not supported", err)
		case errors.Is(err, service.ErrPaymentNotFound):
			common.ErrorResponse(c, http.StatusNotFound, "Payment not found", err)
		default:
			common.ErrorResponse(c, http.StatusInternalServerError, "Failed to process webhook", err)
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// getUserID JWT에서 사용자 ID 추출
func (h *PaymentHandler) getUserID(c *gin.Context) (uint64, error) {
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

// getPaymentID 경로에서 결제 ID 추출
func (h *PaymentHandler) getPaymentID(c *gin.Context) (uint64, error) {
	idStr := c.Param("id")
	return strconv.ParseUint(idStr, 10, 64)
}
