package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/repository"
	pkglogger "github.com/damoang/angple-backend/pkg/logger"
	"gorm.io/gorm"
)

// Payment status constants
const (
	paymentStatusConfirmed = "confirmed"
	paymentStatusCanceled  = "canceled"
	paymentStatusActive    = "active"
)

// PaymentService handles payment processing with Toss and Stripe
type PaymentService struct {
	paymentRepo *repository.PaymentRepository
	subRepo     *repository.SubscriptionRepository
	db          *gorm.DB

	// Toss Payments
	tossSecretKey string
	tossBaseURL   string

	// Stripe
	stripeSecretKey string
}

// PaymentConfig holds payment provider configuration
type PaymentConfig struct {
	TossSecretKey   string
	StripeSecretKey string
}

// NewPaymentService creates a new PaymentService
func NewPaymentService(paymentRepo *repository.PaymentRepository, subRepo *repository.SubscriptionRepository, db *gorm.DB, cfg PaymentConfig) *PaymentService {
	return &PaymentService{
		paymentRepo:     paymentRepo,
		subRepo:         subRepo,
		db:              db,
		tossSecretKey:   cfg.TossSecretKey,
		tossBaseURL:     "https://api.tosspayments.com/v1",
		stripeSecretKey: cfg.StripeSecretKey,
	}
}

// --- Toss Payments ---

// CreateTossPayment creates a pending payment record for Toss checkout
func (s *PaymentService) CreateTossPayment(ctx context.Context, siteID, userID, description string, amount int) (*domain.Payment, error) {
	orderID := generateOrderID()
	idempotencyKey := generateIdempotencyKey()

	payment := &domain.Payment{
		OrderID:        orderID,
		SiteID:         siteID,
		UserID:         userID,
		Provider:       domain.PaymentProviderToss,
		Amount:         amount,
		Currency:       "KRW",
		Status:         "pending",
		Description:    description,
		IdempotencyKey: idempotencyKey,
	}

	if err := s.paymentRepo.Create(ctx, payment); err != nil {
		return nil, fmt.Errorf("create payment: %w", err)
	}

	return payment, nil
}

// ConfirmTossPayment confirms a Toss payment after frontend checkout
func (s *PaymentService) ConfirmTossPayment(ctx context.Context, req *domain.TossConfirmRequest) (*domain.Payment, error) {
	// Find existing payment
	payment, err := s.paymentRepo.FindByOrderID(ctx, req.OrderID)
	if err != nil {
		return nil, fmt.Errorf("payment not found: %w", err)
	}

	// Idempotency: already confirmed
	if payment.Status == paymentStatusConfirmed {
		return payment, nil
	}

	// Verify amount matches
	if payment.Amount != req.Amount {
		return nil, fmt.Errorf("amount mismatch: expected %d, got %d", payment.Amount, req.Amount)
	}

	// Call Toss API to confirm
	body := fmt.Sprintf(`{"paymentKey":"%s","orderId":"%s","amount":%d}`, req.PaymentKey, req.OrderID, req.Amount)
	tossResp, err := s.tossRequest("POST", "/payments/confirm", body)
	if err != nil {
		payment.Status = "failed"
		payment.FailReason = err.Error()
		s.scheduleRetry(payment)
		if updateErr := s.paymentRepo.Update(ctx, payment); updateErr != nil {
			return nil, fmt.Errorf("toss confirm failed: %w; update also failed: %w", err, updateErr)
		}
		return nil, fmt.Errorf("toss confirm failed: %w", err)
	}

	// Update payment
	now := time.Now()
	payment.ExternalPayID = req.PaymentKey
	payment.Status = paymentStatusConfirmed
	payment.ConfirmedAt = &now
	if method, ok := tossResp["method"].(string); ok {
		payment.Method = method
	}

	if err := s.paymentRepo.Update(ctx, payment); err != nil {
		return nil, err
	}

	// Update subscription/invoice status
	s.onPaymentConfirmed(ctx, payment)

	pkglogger.GetLogger().Info().
		Str("order_id", payment.OrderID).
		Int("amount", payment.Amount).
		Msg("toss payment confirmed")

	return payment, nil
}

// HandleTossWebhook processes Toss Payments webhook events
func (s *PaymentService) HandleTossWebhook(ctx context.Context, payload *domain.TossWebhookPayload) error {
	pkglogger.GetLogger().Info().
		Str("event_type", payload.EventType).
		Msg("toss webhook received")

	switch payload.EventType {
	case "PAYMENT_STATUS_CHANGED":
		return s.handleTossStatusChange(ctx, payload.Data)
	default:
		pkglogger.GetLogger().Warn().
			Str("event_type", payload.EventType).
			Msg("unhandled toss webhook event")
	}
	return nil
}

func (s *PaymentService) handleTossStatusChange(ctx context.Context, data map[string]interface{}) error {
	paymentKey, _ := data["paymentKey"].(string) //nolint:errcheck // type assertion, not error
	status, _ := data["status"].(string)         //nolint:errcheck // type assertion, not error

	if paymentKey == "" {
		return fmt.Errorf("missing paymentKey in webhook")
	}

	payment, err := s.paymentRepo.FindByExternalID(ctx, paymentKey)
	if err != nil {
		return fmt.Errorf("payment not found for key %s: %w", paymentKey, err)
	}

	switch status {
	case "DONE":
		now := time.Now()
		payment.Status = paymentStatusConfirmed
		payment.ConfirmedAt = &now
		s.onPaymentConfirmed(ctx, payment)
	case "CANCELED":
		payment.Status = paymentStatusCanceled
	case "ABORTED", "EXPIRED":
		payment.Status = "failed"
		payment.FailReason = status
	}

	return s.paymentRepo.Update(ctx, payment)
}

// --- Stripe ---

// CreateStripeCheckout creates a Stripe checkout session
func (s *PaymentService) CreateStripeCheckout(ctx context.Context, req *domain.StripeCheckoutRequest, userID string) (map[string]string, error) {
	orderID := generateOrderID()

	// Create pending payment
	payment := &domain.Payment{
		OrderID:        orderID,
		SiteID:         req.SiteID,
		UserID:         userID,
		Provider:       domain.PaymentProviderStripe,
		Amount:         0, // Set by Stripe
		Currency:       "USD",
		Status:         "pending",
		Description:    fmt.Sprintf("Plan: %s", req.Plan),
		IdempotencyKey: generateIdempotencyKey(),
	}

	if err := s.paymentRepo.Create(ctx, payment); err != nil {
		return nil, err
	}

	// Call Stripe API to create checkout session
	body := fmt.Sprintf("mode=subscription&success_url=%s&cancel_url=%s&metadata[order_id]=%s&metadata[site_id]=%s",
		req.SuccessURL, req.CancelURL, orderID, req.SiteID)

	resp, err := s.stripeRequest("POST", "/v1/checkout/sessions", body)
	if err != nil {
		return nil, fmt.Errorf("stripe checkout creation failed: %w", err)
	}

	sessionID, _ := resp["id"].(string)   //nolint:errcheck // type assertion, not error
	sessionURL, _ := resp["url"].(string) //nolint:errcheck // type assertion, not error

	return map[string]string{
		"session_id":   sessionID,
		"checkout_url": sessionURL,
		"order_id":     orderID,
	}, nil
}

// HandleStripeWebhook processes Stripe webhook events
func (s *PaymentService) HandleStripeWebhook(ctx context.Context, eventType string, data map[string]interface{}) error {
	pkglogger.GetLogger().Info().
		Str("event_type", eventType).
		Msg("stripe webhook received")

	switch eventType {
	case "checkout.session.completed":
		return s.handleStripeCheckoutCompleted(ctx, data)
	case "invoice.paid":
		return s.handleStripeInvoicePaid(ctx, data)
	case "invoice.payment_failed":
		return s.handleStripePaymentFailed(ctx, data)
	case "customer.subscription.deleted":
		return s.handleStripeSubscriptionCanceled(ctx, data)
	}
	return nil
}

func (s *PaymentService) handleStripeCheckoutCompleted(ctx context.Context, data map[string]interface{}) error {
	metadata, _ := data["metadata"].(map[string]interface{}) //nolint:errcheck // type assertion, not error
	orderID, _ := metadata["order_id"].(string)              //nolint:errcheck // type assertion, not error

	if orderID == "" {
		return nil
	}

	payment, err := s.paymentRepo.FindByOrderID(ctx, orderID)
	if err != nil {
		return err
	}

	now := time.Now()
	payment.Status = paymentStatusConfirmed
	payment.ConfirmedAt = &now
	if subID, ok := data["subscription"].(string); ok {
		payment.ExternalPayID = subID
	}

	s.onPaymentConfirmed(ctx, payment)
	return s.paymentRepo.Update(ctx, payment)
}

func (s *PaymentService) handleStripeInvoicePaid(_ context.Context, data map[string]interface{}) error {
	subID, _ := data["subscription"].(string) //nolint:errcheck // type assertion, not error
	if subID == "" {
		return nil
	}

	// Update subscription period
	var sub domain.Subscription
	if err := s.db.Where("external_sub_id = ?", subID).First(&sub).Error; err != nil {
		return nil
	}

	sub.Status = paymentStatusActive
	sub.CurrentPeriodStart = time.Now()
	sub.CurrentPeriodEnd = time.Now().AddDate(0, 1, 0)
	return s.db.Save(&sub).Error
}

func (s *PaymentService) handleStripePaymentFailed(_ context.Context, data map[string]interface{}) error {
	subID, _ := data["subscription"].(string) //nolint:errcheck // type assertion, not error
	if subID == "" {
		return nil
	}

	var sub domain.Subscription
	if err := s.db.Where("external_sub_id = ?", subID).First(&sub).Error; err != nil {
		return nil
	}

	sub.Status = "past_due"
	return s.db.Save(&sub).Error
}

func (s *PaymentService) handleStripeSubscriptionCanceled(_ context.Context, data map[string]interface{}) error {
	subID, _ := data["id"].(string) //nolint:errcheck // type assertion, not error
	if subID == "" {
		return nil
	}

	var sub domain.Subscription
	if err := s.db.Where("external_sub_id = ?", subID).First(&sub).Error; err != nil {
		return nil
	}

	now := time.Now()
	sub.Status = paymentStatusCanceled
	sub.CanceledAt = &now
	return s.db.Save(&sub).Error
}

// --- Refund ---

// RefundPayment processes a refund (full or partial)
func (s *PaymentService) RefundPayment(ctx context.Context, req *domain.RefundRequest) (*domain.Payment, error) {
	var payment domain.Payment
	if err := s.db.WithContext(ctx).First(&payment, req.PaymentID).Error; err != nil {
		return nil, fmt.Errorf("payment not found")
	}

	if payment.Status != paymentStatusConfirmed {
		return nil, fmt.Errorf("can only refund confirmed payments")
	}

	refundAmount := req.Amount
	if refundAmount == 0 {
		refundAmount = payment.Amount - payment.RefundedAmount
	}

	if refundAmount <= 0 || refundAmount > (payment.Amount-payment.RefundedAmount) {
		return nil, fmt.Errorf("invalid refund amount")
	}

	switch payment.Provider {
	case domain.PaymentProviderToss:
		body := fmt.Sprintf(`{"cancelReason":"%s","cancelAmount":%d}`, req.Reason, refundAmount)
		_, err := s.tossRequest("POST", fmt.Sprintf("/payments/%s/cancel", payment.ExternalPayID), body)
		if err != nil {
			return nil, fmt.Errorf("toss refund failed: %w", err)
		}

	case domain.PaymentProviderStripe:
		body := fmt.Sprintf("payment_intent=%s&amount=%d&reason=requested_by_customer", payment.ExternalPayID, refundAmount)
		_, err := s.stripeRequest("POST", "/v1/refunds", body)
		if err != nil {
			return nil, fmt.Errorf("stripe refund failed: %w", err)
		}
	}

	payment.RefundedAmount += refundAmount
	if payment.RefundedAmount >= payment.Amount {
		payment.Status = "refunded"
	} else {
		payment.Status = "partial_refunded"
	}

	if err := s.paymentRepo.Update(ctx, &payment); err != nil {
		return nil, err
	}

	pkglogger.GetLogger().Info().
		Str("order_id", payment.OrderID).
		Int("refund_amount", refundAmount).
		Msg("payment refunded")

	return &payment, nil
}

// --- Payments List ---

// ListPayments returns paginated payments for a site
func (s *PaymentService) ListPayments(ctx context.Context, siteID string, page, perPage int) ([]domain.Payment, int64, error) {
	return s.paymentRepo.ListBySiteID(ctx, siteID, page, perPage)
}

// GetPayment returns a single payment by order ID
func (s *PaymentService) GetPayment(ctx context.Context, orderID string) (*domain.Payment, error) {
	return s.paymentRepo.FindByOrderID(ctx, orderID)
}

// --- Internal helpers ---

func (s *PaymentService) onPaymentConfirmed(_ context.Context, payment *domain.Payment) {
	// Update related invoice if exists
	if payment.InvoiceID != nil {
		_ = s.db.Model(&domain.Invoice{}).Where("id = ?", *payment.InvoiceID).Updates(map[string]interface{}{
			"status":  "paid",
			"paid_at": time.Now(),
		}).Error
	}

	// Activate subscription
	_ = s.db.Model(&domain.Subscription{}).Where("site_id = ?", payment.SiteID).Updates(map[string]interface{}{
		"status":           paymentStatusActive,
		"payment_provider": string(payment.Provider),
	}).Error
}

func (s *PaymentService) scheduleRetry(payment *domain.Payment) {
	payment.RetryCount++
	// Exponential backoff: 1h, 4h, 24h
	delays := []time.Duration{1 * time.Hour, 4 * time.Hour, 24 * time.Hour}
	if payment.RetryCount <= len(delays) {
		next := time.Now().Add(delays[payment.RetryCount-1])
		payment.NextRetryAt = &next
	}
}

func (s *PaymentService) tossRequest(method, path, body string) (map[string]interface{}, error) {
	reqURL := s.tossBaseURL + path
	req, err := http.NewRequestWithContext(context.Background(), method, reqURL, strings.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Basic "+basicAuth(s.tossSecretKey, ""))

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read toss response body: %w", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse toss response: %w", err)
	}

	if resp.StatusCode >= 400 {
		errMsg, _ := result["message"].(string) //nolint:errcheck // type assertion, not error
		return nil, fmt.Errorf("toss API error (%d): %s", resp.StatusCode, errMsg)
	}

	return result, nil
}

func (s *PaymentService) stripeRequest(method, path, body string) (map[string]interface{}, error) {
	reqURL := "https://api.stripe.com" + path
	req, err := http.NewRequestWithContext(context.Background(), method, reqURL, strings.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+s.stripeSecretKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read stripe response body: %w", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse stripe response: %w", err)
	}

	if resp.StatusCode >= 400 {
		if errObj, ok := result["error"].(map[string]interface{}); ok {
			msg, _ := errObj["message"].(string) //nolint:errcheck // type assertion, not error
			return nil, fmt.Errorf("stripe API error (%d): %s", resp.StatusCode, msg)
		}
		return nil, fmt.Errorf("stripe API error (%d)", resp.StatusCode)
	}

	return result, nil
}

func generateOrderID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback: use timestamp-only order ID if crypto/rand fails
		return fmt.Sprintf("ORD_%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("ORD_%s_%d", hex.EncodeToString(b[:8]), time.Now().UnixMilli())
}

func generateIdempotencyKey() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback: use timestamp-based key if crypto/rand fails
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func basicAuth(username, password string) string {
	return base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
}
