package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/internal/service"
	"github.com/gin-gonic/gin"
)

// PaymentHandler handles payment-related endpoints
type PaymentHandler struct {
	paymentService *service.PaymentService
}

// NewPaymentHandler creates a new PaymentHandler
func NewPaymentHandler(paymentService *service.PaymentService) *PaymentHandler {
	return &PaymentHandler{paymentService: paymentService}
}

// CreateTossPayment creates a pending payment for Toss checkout
// POST /api/v2/payments/toss
func (h *PaymentHandler) CreateTossPayment(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == "" {
		common.ErrorResponse(c, http.StatusUnauthorized, "Authentication required", nil)
		return
	}

	var req struct {
		SiteID      string `json:"site_id" binding:"required"`
		Description string `json:"description" binding:"required"`
		Amount      int    `json:"amount" binding:"required,min=100"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request", nil)
		return
	}

	payment, err := h.paymentService.CreateTossPayment(c.Request.Context(), req.SiteID, userID, req.Description, req.Amount)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, err.Error(), nil)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": payment})
}

// ConfirmTossPayment confirms a Toss payment after frontend checkout
// POST /api/v2/payments/toss/confirm
func (h *PaymentHandler) ConfirmTossPayment(c *gin.Context) {
	var req domain.TossConfirmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request", nil)
		return
	}

	payment, err := h.paymentService.ConfirmTossPayment(c.Request.Context(), &req)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": payment})
}

// TossWebhook handles Toss Payments webhook
// POST /api/v2/payments/toss/webhook
func (h *PaymentHandler) TossWebhook(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}

	var payload domain.TossWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		c.Status(http.StatusBadRequest)
		return
	}

	if err := h.paymentService.HandleTossWebhook(c.Request.Context(), &payload); err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	c.Status(http.StatusOK)
}

// CreateStripeCheckout creates a Stripe checkout session
// POST /api/v2/payments/stripe/checkout
func (h *PaymentHandler) CreateStripeCheckout(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == "" {
		common.ErrorResponse(c, http.StatusUnauthorized, "Authentication required", nil)
		return
	}

	var req domain.StripeCheckoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request", nil)
		return
	}

	result, err := h.paymentService.CreateStripeCheckout(c.Request.Context(), &req, userID)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, err.Error(), nil)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// StripeWebhook handles Stripe webhook events
// POST /api/v2/payments/stripe/webhook
func (h *PaymentHandler) StripeWebhook(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}

	var event struct {
		Type string `json:"type"`
		Data struct {
			Object map[string]interface{} `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &event); err != nil {
		c.Status(http.StatusBadRequest)
		return
	}

	if err := h.paymentService.HandleStripeWebhook(c.Request.Context(), event.Type, event.Data.Object); err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	c.Status(http.StatusOK)
}

// RefundPayment processes a refund
// POST /api/v2/payments/refund
func (h *PaymentHandler) RefundPayment(c *gin.Context) {
	var req domain.RefundRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request", nil)
		return
	}

	payment, err := h.paymentService.RefundPayment(c.Request.Context(), &req)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": payment})
}

// ListPayments lists payments for a site
// GET /api/v2/payments?site_id=xxx&page=1&per_page=20
func (h *PaymentHandler) ListPayments(c *gin.Context) {
	siteID := c.Query("site_id")
	if siteID == "" {
		common.ErrorResponse(c, http.StatusBadRequest, "site_id is required", nil)
		return
	}

	page := 1
	if val, err := strconv.Atoi(c.DefaultQuery("page", "1")); err == nil {
		page = val
	}
	perPage := 20
	if val, err := strconv.Atoi(c.DefaultQuery("per_page", "20")); err == nil {
		perPage = val
	}
	if page < 1 {
		page = 1
	}

	payments, total, err := h.paymentService.ListPayments(c.Request.Context(), siteID, page, perPage)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to list payments", nil)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    payments,
		"meta":    gin.H{"total": total, "page": page, "per_page": perPage},
	})
}

// GetPayment returns a single payment by order ID
// GET /api/v2/payments/:order_id
func (h *PaymentHandler) GetPayment(c *gin.Context) {
	orderID := c.Param("order_id")

	payment, err := h.paymentService.GetPayment(c.Request.Context(), orderID)
	if err != nil {
		common.ErrorResponse(c, http.StatusNotFound, "Payment not found", nil)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": payment})
}
