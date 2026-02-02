package domain

import "time"

// PaymentProvider supported payment providers
type PaymentProvider string

const (
	PaymentProviderToss   PaymentProvider = "toss"
	PaymentProviderStripe PaymentProvider = "stripe"
)

// Payment represents a single payment transaction
type Payment struct {
	ID              int64           `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	OrderID         string          `gorm:"column:order_id;uniqueIndex;size:64" json:"order_id"`
	SiteID          string          `gorm:"column:site_id;index" json:"site_id"`
	UserID          string          `gorm:"column:user_id;index" json:"user_id"`
	InvoiceID       *int64          `gorm:"column:invoice_id" json:"invoice_id,omitempty"`
	Provider        PaymentProvider `gorm:"column:provider" json:"provider"`
	ExternalPayID   string          `gorm:"column:external_pay_id;index" json:"external_pay_id"` // toss paymentKey / stripe payment_intent
	Amount          int             `gorm:"column:amount" json:"amount"`
	Currency        string          `gorm:"column:currency;default:KRW" json:"currency"`
	Status          string          `gorm:"column:status;default:pending" json:"status"` // pending, confirmed, failed, canceled, refunded, partial_refunded
	Method          string          `gorm:"column:method" json:"method"`                 // card, transfer, virtual_account
	Description     string          `gorm:"column:description" json:"description"`
	FailReason      string          `gorm:"column:fail_reason" json:"fail_reason,omitempty"`
	RefundedAmount  int             `gorm:"column:refunded_amount;default:0" json:"refunded_amount"`
	IdempotencyKey  string          `gorm:"column:idempotency_key;uniqueIndex;size:64" json:"-"`
	Metadata        string          `gorm:"column:metadata;type:text" json:"metadata,omitempty"` // JSON
	RetryCount      int             `gorm:"column:retry_count;default:0" json:"retry_count"`
	NextRetryAt     *time.Time      `gorm:"column:next_retry_at" json:"next_retry_at,omitempty"`
	ConfirmedAt     *time.Time      `gorm:"column:confirmed_at" json:"confirmed_at,omitempty"`
	CreatedAt       time.Time       `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time       `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (Payment) TableName() string {
	return "payments"
}

// RefundRequest represents a refund request
type RefundRequest struct {
	PaymentID int64  `json:"payment_id" binding:"required"`
	Amount    int    `json:"amount"` // 0 = full refund
	Reason    string `json:"reason" binding:"required"`
}

// TossConfirmRequest is sent by the frontend after Toss checkout
type TossConfirmRequest struct {
	PaymentKey string `json:"paymentKey" binding:"required"`
	OrderID    string `json:"orderId" binding:"required"`
	Amount     int    `json:"amount" binding:"required"`
}

// TossWebhookPayload is the payload from Toss Payments webhook
type TossWebhookPayload struct {
	EventType string                 `json:"eventType"`
	Data      map[string]interface{} `json:"data"`
}

// StripeCheckoutRequest for creating a Stripe checkout session
type StripeCheckoutRequest struct {
	SiteID     string `json:"site_id" binding:"required"`
	Plan       string `json:"plan" binding:"required"`
	SuccessURL string `json:"success_url" binding:"required"`
	CancelURL  string `json:"cancel_url" binding:"required"`
}

// PaymentSummary for listing payments
type PaymentSummary struct {
	ID            int64  `json:"id"`
	OrderID       string `json:"order_id"`
	Provider      string `json:"provider"`
	Amount        int    `json:"amount"`
	Currency      string `json:"currency"`
	Status        string `json:"status"`
	Method        string `json:"method"`
	Description   string `json:"description"`
	ConfirmedAt   string `json:"confirmed_at,omitempty"`
	CreatedAt     string `json:"created_at"`
}
