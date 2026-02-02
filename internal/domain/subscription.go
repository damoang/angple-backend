package domain

import "time"

// Subscription represents a tenant's subscription to a plan
type Subscription struct {
	CreatedAt          time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt          time.Time  `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
	CurrentPeriodStart time.Time  `gorm:"column:current_period_start" json:"current_period_start"`
	CurrentPeriodEnd   time.Time  `gorm:"column:current_period_end" json:"current_period_end"`
	CanceledAt         *time.Time `gorm:"column:canceled_at" json:"canceled_at,omitempty"`

	SiteID          string `gorm:"column:site_id;uniqueIndex" json:"site_id"`
	Plan            string `gorm:"column:plan" json:"plan"`                                   // free, pro, business, enterprise
	Status          string `gorm:"column:status;default:active" json:"status"`                // active, past_due, canceled, trialing
	PaymentProvider string `gorm:"column:payment_provider" json:"payment_provider"`           // stripe, toss, manual
	ExternalSubID   string `gorm:"column:external_sub_id" json:"external_sub_id"`             // Stripe/Toss subscription ID
	BillingCycle    string `gorm:"column:billing_cycle;default:monthly" json:"billing_cycle"` // monthly, yearly

	ID              int64 `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	MonthlyPriceKRW int   `gorm:"column:monthly_price_krw;default:0" json:"monthly_price_krw"`
}

func (Subscription) TableName() string {
	return "subscriptions"
}

// Invoice represents a billing invoice
type Invoice struct {
	CreatedAt   time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	PaidAt      *time.Time `gorm:"column:paid_at" json:"paid_at,omitempty"`
	PeriodStart time.Time  `gorm:"column:period_start" json:"period_start"`
	PeriodEnd   time.Time  `gorm:"column:period_end" json:"period_end"`

	SiteID          string `gorm:"column:site_id;index" json:"site_id"`
	Status          string `gorm:"column:status;default:pending" json:"status"` // pending, paid, failed, refunded
	PaymentProvider string `gorm:"column:payment_provider" json:"payment_provider"`
	ExternalInvID   string `gorm:"column:external_inv_id" json:"external_inv_id"`
	Description     string `gorm:"column:description" json:"description"`
	Currency        string `gorm:"column:currency;default:KRW" json:"currency"`

	ID        int64 `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	AmountKRW int   `gorm:"column:amount_krw" json:"amount_krw"`
}

func (Invoice) TableName() string {
	return "invoices"
}

// ProvisionRequest is the request body for one-click community creation
type ProvisionRequest struct {
	Subdomain  string  `json:"subdomain" binding:"required,min=3,max=50"`
	SiteName   string  `json:"site_name" binding:"required,min=1,max=100"`
	OwnerEmail string  `json:"owner_email" binding:"required,email"`
	Plan       string  `json:"plan" binding:"required,oneof=free pro business enterprise"`
	Theme      *string `json:"theme,omitempty"`
}

// ProvisionResponse is the response after provisioning a community
type ProvisionResponse struct {
	SiteID      string `json:"site_id"`
	Subdomain   string `json:"subdomain"`
	SiteURL     string `json:"site_url"`
	Plan        string `json:"plan"`
	DBStrategy  string `json:"db_strategy"`
	TrialEndsAt string `json:"trial_ends_at,omitempty"`
	Message     string `json:"message"`
}

// SubscriptionResponse is the response for subscription info
type SubscriptionResponse struct {
	Plan               string `json:"plan"`
	Status             string `json:"status"`
	BillingCycle       string `json:"billing_cycle"`
	MonthlyPriceKRW    int    `json:"monthly_price_krw"`
	CurrentPeriodStart string `json:"current_period_start"`
	CurrentPeriodEnd   string `json:"current_period_end"`
	CanceledAt         string `json:"canceled_at,omitempty"`
}

// ChangePlanRequest for upgrading/downgrading
type ChangePlanRequest struct {
	Plan         string `json:"plan" binding:"required,oneof=free pro business enterprise"`
	BillingCycle string `json:"billing_cycle" binding:"omitempty,oneof=monthly yearly"`
}

// PlanPricing defines pricing for each plan
type PlanPricing struct {
	Plan       string `json:"plan"`
	MonthlyKRW int    `json:"monthly_krw"`
	YearlyKRW  int    `json:"yearly_krw"`
	TrialDays  int    `json:"trial_days"`
}
