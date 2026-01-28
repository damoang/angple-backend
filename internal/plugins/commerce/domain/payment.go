package domain

import (
	"time"
)

// PaymentStatus 결제 상태
type PaymentStatus string

const (
	PaymentStatusPending          PaymentStatus = "pending"
	PaymentStatusReady            PaymentStatus = "ready"            // 가상계좌 입금 대기
	PaymentStatusPaid             PaymentStatus = "paid"             // 결제 완료
	PaymentStatusCancelled        PaymentStatus = "cancelled"        // 전체 취소
	PaymentStatusPartialCancelled PaymentStatus = "partial_cancelled" // 부분 취소
	PaymentStatusFailed           PaymentStatus = "failed"           // 실패
)

// PGProvider PG사 구분
type PGProvider string

const (
	PGProviderInicis      PGProvider = "inicis"      // KG이니시스
	PGProviderTossPayments PGProvider = "tosspayments" // 토스페이먼츠
	PGProviderKakaoPay    PGProvider = "kakaopay"    // 카카오페이 (Phase 8)
)

// PaymentMethod 결제 수단
type PaymentMethod string

const (
	PaymentMethodCard    PaymentMethod = "card"    // 신용/체크카드
	PaymentMethodBank    PaymentMethod = "bank"    // 계좌이체
	PaymentMethodVirtual PaymentMethod = "virtual" // 가상계좌
	PaymentMethodPhone   PaymentMethod = "phone"   // 휴대폰 결제
)

// Payment 결제 엔티티
type Payment struct {
	ID      uint64 `gorm:"primaryKey" json:"id"`
	OrderID uint64 `gorm:"column:order_id;not null" json:"order_id"`

	// PG 정보
	PGProvider PGProvider `gorm:"column:pg_provider;size:50;not null" json:"pg_provider"`
	PGTID      string     `gorm:"column:pg_tid;size:100" json:"pg_tid,omitempty"`
	PGOrderID  string     `gorm:"column:pg_order_id;size:100" json:"pg_order_id,omitempty"`

	// 결제 정보
	PaymentMethod PaymentMethod `gorm:"column:payment_method;size:50" json:"payment_method,omitempty"`
	Amount        float64       `gorm:"type:decimal(12,2);not null" json:"amount"`
	Currency      string        `gorm:"size:3;default:'KRW'" json:"currency"`

	// 상태
	Status PaymentStatus `gorm:"size:20;default:'pending'" json:"status"`

	// 카드 정보 (마스킹)
	CardCompany  string `gorm:"column:card_company;size:50" json:"card_company,omitempty"`
	CardNumber   string `gorm:"column:card_number;size:20" json:"card_number,omitempty"`
	CardType     string `gorm:"column:card_type;size:20" json:"card_type,omitempty"`
	InstallMonth int    `gorm:"column:install_month" json:"install_month,omitempty"`

	// 가상계좌 정보
	VBankName   string     `gorm:"column:vbank_name;size:50" json:"vbank_name,omitempty"`
	VBankNumber string     `gorm:"column:vbank_number;size:50" json:"vbank_number,omitempty"`
	VBankHolder string     `gorm:"column:vbank_holder;size:50" json:"vbank_holder,omitempty"`
	VBankDue    *time.Time `gorm:"column:vbank_due" json:"vbank_due,omitempty"`

	// 수수료
	PGFee *float64 `gorm:"column:pg_fee;type:decimal(12,2)" json:"pg_fee,omitempty"`

	// 취소/환불
	CancelledAmount float64    `gorm:"column:cancelled_amount;type:decimal(12,2);default:0" json:"cancelled_amount"`
	CancelReason    string     `gorm:"column:cancel_reason;size:255" json:"cancel_reason,omitempty"`
	CancelledAt     *time.Time `gorm:"column:cancelled_at" json:"cancelled_at,omitempty"`

	// 메타
	RawResponse string `gorm:"column:raw_response;type:json" json:"-"`
	MetaData    string `gorm:"column:meta_data;type:json" json:"meta_data,omitempty"`

	// 타임스탬프
	PaidAt    *time.Time `gorm:"column:paid_at" json:"paid_at,omitempty"`
	CreatedAt time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt time.Time  `gorm:"column:updated_at" json:"updated_at"`

	// Relations
	Order *Order `gorm:"foreignKey:OrderID" json:"-"`
}

// TableName GORM 테이블명
func (Payment) TableName() string {
	return "commerce_payments"
}

// PreparePaymentRequest 결제 준비 요청 DTO
type PreparePaymentRequest struct {
	OrderID       uint64 `json:"order_id" binding:"required"`
	PGProvider    string `json:"pg_provider" binding:"required,oneof=inicis tosspayments kakaopay"`
	PaymentMethod string `json:"payment_method" binding:"required,oneof=card bank virtual phone"`
	ReturnURL     string `json:"return_url" binding:"required,url"`
	CancelURL     string `json:"cancel_url" binding:"omitempty,url"`
}

// PreparePaymentResponse 결제 준비 응답 DTO
type PreparePaymentResponse struct {
	PaymentID   uint64 `json:"payment_id"`
	OrderNumber string `json:"order_number"`
	Amount      float64 `json:"amount"`
	Currency    string `json:"currency"`

	// PG별 응답
	PGProvider   string `json:"pg_provider"`
	PGOrderID    string `json:"pg_order_id"`
	RedirectURL  string `json:"redirect_url,omitempty"`  // 리다이렉트 URL (PC)
	MobileURL    string `json:"mobile_url,omitempty"`    // 모바일 URL
	AppScheme    string `json:"app_scheme,omitempty"`    // 앱 스킴

	// 추가 데이터
	MerchantID   string `json:"merchant_id,omitempty"`
	Signature    string `json:"signature,omitempty"`
	Timestamp    string `json:"timestamp,omitempty"`
	ExtraData    map[string]string `json:"extra_data,omitempty"`
}

// CompletePaymentRequest 결제 완료 요청 DTO
type CompletePaymentRequest struct {
	PaymentID    uint64 `json:"payment_id" binding:"required"`
	PGProvider   string `json:"pg_provider" binding:"required"`
	PGTID        string `json:"pg_tid" binding:"required"`
	PGOrderID    string `json:"pg_order_id" binding:"required"`
	Amount       float64 `json:"amount" binding:"required"`

	// 카드 결제 시
	CardCompany  string `json:"card_company,omitempty"`
	CardNumber   string `json:"card_number,omitempty"`
	CardType     string `json:"card_type,omitempty"`
	InstallMonth int    `json:"install_month,omitempty"`

	// 가상계좌 결제 시
	VBankName    string `json:"vbank_name,omitempty"`
	VBankNumber  string `json:"vbank_number,omitempty"`
	VBankHolder  string `json:"vbank_holder,omitempty"`
	VBankDue     string `json:"vbank_due,omitempty"`
}

// CancelPaymentRequest 결제 취소 요청 DTO
type CancelPaymentRequest struct {
	PaymentID    uint64  `json:"payment_id" binding:"required"`
	CancelAmount float64 `json:"cancel_amount" binding:"omitempty,gt=0"`
	CancelReason string  `json:"cancel_reason" binding:"required,max=255"`
}

// PaymentResponse 결제 응답 DTO
type PaymentResponse struct {
	ID            uint64     `json:"id"`
	OrderID       uint64     `json:"order_id"`
	OrderNumber   string     `json:"order_number,omitempty"`
	PGProvider    string     `json:"pg_provider"`
	PGTID         string     `json:"pg_tid,omitempty"`
	PaymentMethod string     `json:"payment_method,omitempty"`
	Amount        float64    `json:"amount"`
	Currency      string     `json:"currency"`
	Status        string     `json:"status"`
	CardCompany   string     `json:"card_company,omitempty"`
	CardNumber    string     `json:"card_number,omitempty"`
	InstallMonth  int        `json:"install_month,omitempty"`
	VBankName     string     `json:"vbank_name,omitempty"`
	VBankNumber   string     `json:"vbank_number,omitempty"`
	VBankHolder   string     `json:"vbank_holder,omitempty"`
	VBankDue      *time.Time `json:"vbank_due,omitempty"`
	PaidAt        *time.Time `json:"paid_at,omitempty"`
	CancelledAt   *time.Time `json:"cancelled_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

// ToResponse Payment를 PaymentResponse로 변환
func (p *Payment) ToResponse() *PaymentResponse {
	response := &PaymentResponse{
		ID:            p.ID,
		OrderID:       p.OrderID,
		PGProvider:    string(p.PGProvider),
		PGTID:         p.PGTID,
		PaymentMethod: string(p.PaymentMethod),
		Amount:        p.Amount,
		Currency:      p.Currency,
		Status:        string(p.Status),
		CardCompany:   p.CardCompany,
		CardNumber:    p.CardNumber,
		InstallMonth:  p.InstallMonth,
		VBankName:     p.VBankName,
		VBankNumber:   p.VBankNumber,
		VBankHolder:   p.VBankHolder,
		VBankDue:      p.VBankDue,
		PaidAt:        p.PaidAt,
		CancelledAt:   p.CancelledAt,
		CreatedAt:     p.CreatedAt,
	}

	if p.Order != nil {
		response.OrderNumber = p.Order.OrderNumber
	}

	return response
}

// WebhookPayload PG 웹훅 페이로드
type WebhookPayload struct {
	Provider     PGProvider `json:"provider"`
	EventType    string     `json:"event_type"`
	PGTID        string     `json:"pg_tid"`
	PGOrderID    string     `json:"pg_order_id"`
	Amount       float64    `json:"amount"`
	Status       string     `json:"status"`
	RawData      string     `json:"raw_data"`
}
