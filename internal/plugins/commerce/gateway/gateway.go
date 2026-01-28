package gateway

import (
	"context"
	"errors"

	"github.com/damoang/angple-backend/internal/plugins/commerce/domain"
)

// PaymentGateway 결제 게이트웨이 인터페이스
type PaymentGateway interface {
	// Provider PG사 이름 반환
	Provider() domain.PGProvider

	// Prepare 결제 준비 (결제창 호출 전)
	Prepare(ctx context.Context, req *PrepareRequest) (*PrepareResponse, error)

	// Complete 결제 완료 처리 (결제창 완료 후)
	Complete(ctx context.Context, req *CompleteRequest) (*CompleteResponse, error)

	// Cancel 결제 취소
	Cancel(ctx context.Context, req *CancelRequest) (*CancelResponse, error)

	// HandleWebhook 웹훅 처리
	HandleWebhook(ctx context.Context, payload []byte) (*WebhookResult, error)

	// Verify 결제 검증 (금액 일치 등)
	Verify(ctx context.Context, pgTID string, amount float64) error
}

// PrepareRequest 결제 준비 요청
type PrepareRequest struct {
	OrderID       uint64
	OrderNumber   string
	Amount        float64
	Currency      string
	ProductName   string
	BuyerName     string
	BuyerEmail    string
	BuyerPhone    string
	PaymentMethod domain.PaymentMethod
	ReturnURL     string
	CancelURL     string
	NotifyURL     string // 웹훅 URL
}

// PrepareResponse 결제 준비 응답
type PrepareResponse struct {
	PGOrderID   string
	RedirectURL string
	MobileURL   string
	AppScheme   string
	MerchantID  string
	Signature   string
	Timestamp   string
	ExtraData   map[string]string
}

// CompleteRequest 결제 완료 요청
type CompleteRequest struct {
	PGTID       string
	PGOrderID   string
	Amount      float64
	RawResponse string
}

// CompleteResponse 결제 완료 응답
type CompleteResponse struct {
	Success       bool
	PGTID         string
	PGOrderID     string
	Amount        float64
	PaymentMethod domain.PaymentMethod
	Status        domain.PaymentStatus

	// 카드 결제
	CardCompany  string
	CardNumber   string
	CardType     string
	InstallMonth int

	// 가상계좌
	VBankName   string
	VBankNumber string
	VBankHolder string
	VBankDue    string

	// 수수료
	PGFee float64

	// 원본 응답
	RawResponse string
}

// CancelRequest 결제 취소 요청
type CancelRequest struct {
	PGTID        string
	PGOrderID    string
	CancelAmount float64
	TotalAmount  float64
	CancelReason string
}

// CancelResponse 결제 취소 응답
type CancelResponse struct {
	Success          bool
	CancelledAmount  float64
	RemainingAmount  float64
	CancelledAt      string
	RawResponse      string
}

// WebhookResult 웹훅 처리 결과
type WebhookResult struct {
	EventType    string
	PGTID        string
	PGOrderID    string
	Amount       float64
	Status       domain.PaymentStatus
	VBankName    string
	VBankNumber  string
	VBankHolder  string
	RawData      string
}

// Gateway 에러 정의
var (
	ErrGatewayNotFound     = errors.New("payment gateway not found")
	ErrInvalidAmount       = errors.New("invalid payment amount")
	ErrPaymentFailed       = errors.New("payment failed")
	ErrPaymentNotFound     = errors.New("payment not found")
	ErrAlreadyPaid         = errors.New("payment already completed")
	ErrAlreadyCancelled    = errors.New("payment already cancelled")
	ErrCancelNotAllowed    = errors.New("payment cancel not allowed")
	ErrInvalidWebhook      = errors.New("invalid webhook payload")
	ErrWebhookVerification = errors.New("webhook verification failed")
)

// GatewayManager 게이트웨이 관리자
type GatewayManager struct {
	gateways map[domain.PGProvider]PaymentGateway
}

// NewGatewayManager 게이트웨이 관리자 생성
func NewGatewayManager() *GatewayManager {
	return &GatewayManager{
		gateways: make(map[domain.PGProvider]PaymentGateway),
	}
}

// Register 게이트웨이 등록
func (m *GatewayManager) Register(gateway PaymentGateway) {
	m.gateways[gateway.Provider()] = gateway
}

// Get 게이트웨이 조회
func (m *GatewayManager) Get(provider domain.PGProvider) (PaymentGateway, error) {
	gateway, ok := m.gateways[provider]
	if !ok {
		return nil, ErrGatewayNotFound
	}
	return gateway, nil
}

// List 등록된 게이트웨이 목록
func (m *GatewayManager) List() []domain.PGProvider {
	providers := make([]domain.PGProvider, 0, len(m.gateways))
	for provider := range m.gateways {
		providers = append(providers, provider)
	}
	return providers
}
