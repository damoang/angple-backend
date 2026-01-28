package gateway

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/damoang/angple-backend/internal/plugins/commerce/domain"
)

// TossPaymentsConfig 토스페이먼츠 설정
type TossPaymentsConfig struct {
	ClientKey  string // 클라이언트 키 (프론트엔드용)
	SecretKey  string // 시크릿 키 (서버용)
	IsSandbox  bool   // 테스트 모드
}

// TossPaymentsGateway 토스페이먼츠 게이트웨이 구현
type TossPaymentsGateway struct {
	config     *TossPaymentsConfig
	httpClient *http.Client
}

// NewTossPaymentsGateway 생성자
func NewTossPaymentsGateway(config *TossPaymentsConfig) *TossPaymentsGateway {
	return &TossPaymentsGateway{
		config: config,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Provider PG사 이름 반환
func (g *TossPaymentsGateway) Provider() domain.PGProvider {
	return domain.PGProviderTossPayments
}

// Prepare 결제 준비
func (g *TossPaymentsGateway) Prepare(ctx context.Context, req *PrepareRequest) (*PrepareResponse, error) {
	// 토스페이먼츠는 프론트엔드에서 결제창 호출
	// 서버에서는 클라이언트 키와 주문 정보 반환
	pgOrderID := fmt.Sprintf("TOSS_%s_%d", req.OrderNumber, time.Now().UnixNano()%1000000)

	return &PrepareResponse{
		PGOrderID:  pgOrderID,
		MerchantID: g.config.ClientKey,
		ExtraData: map[string]string{
			"clientKey":     g.config.ClientKey,
			"orderId":       pgOrderID,
			"amount":        fmt.Sprintf("%.0f", req.Amount),
			"orderName":     req.ProductName,
			"customerName":  req.BuyerName,
			"customerEmail": req.BuyerEmail,
			"successUrl":    req.ReturnURL,
			"failUrl":       req.CancelURL,
		},
	}, nil
}

// Complete 결제 완료 처리
func (g *TossPaymentsGateway) Complete(ctx context.Context, req *CompleteRequest) (*CompleteResponse, error) {
	// 토스페이먼츠 결제 승인 API 호출
	confirmURL := g.getAPIURL() + "/v1/payments/confirm"

	payload := map[string]interface{}{
		"paymentKey": req.PGTID,
		"orderId":    req.PGOrderID,
		"amount":     req.Amount,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", confirmURL, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	// Basic 인증 헤더 설정
	auth := base64.StdEncoding.EncodeToString([]byte(g.config.SecretKey + ":"))
	httpReq.Header.Set("Authorization", "Basic "+auth)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		var errResp TossErrorResponse
		if err := json.Unmarshal(respBody, &errResp); err == nil {
			return nil, fmt.Errorf("payment failed: %s - %s", errResp.Code, errResp.Message)
		}
		return nil, ErrPaymentFailed
	}

	var tossResp TossPaymentResponse
	if err := json.Unmarshal(respBody, &tossResp); err != nil {
		return nil, err
	}

	return g.convertToCompleteResponse(&tossResp, string(respBody)), nil
}

// Cancel 결제 취소
func (g *TossPaymentsGateway) Cancel(ctx context.Context, req *CancelRequest) (*CancelResponse, error) {
	cancelURL := g.getAPIURL() + fmt.Sprintf("/v1/payments/%s/cancel", req.PGTID)

	payload := map[string]interface{}{
		"cancelReason": req.CancelReason,
	}

	// 부분 취소인 경우
	if req.CancelAmount > 0 && req.CancelAmount < req.TotalAmount {
		payload["cancelAmount"] = req.CancelAmount
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", cancelURL, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	auth := base64.StdEncoding.EncodeToString([]byte(g.config.SecretKey + ":"))
	httpReq.Header.Set("Authorization", "Basic "+auth)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		var errResp TossErrorResponse
		if err := json.Unmarshal(respBody, &errResp); err == nil {
			return nil, fmt.Errorf("cancel failed: %s - %s", errResp.Code, errResp.Message)
		}
		return nil, ErrCancelNotAllowed
	}

	var tossResp TossPaymentResponse
	if err := json.Unmarshal(respBody, &tossResp); err != nil {
		return nil, err
	}

	// 취소 금액 계산
	cancelledAmount := 0.0
	for _, cancel := range tossResp.Cancels {
		cancelledAmount += cancel.CancelAmount
	}

	return &CancelResponse{
		Success:         true,
		CancelledAmount: cancelledAmount,
		RemainingAmount: tossResp.TotalAmount - cancelledAmount,
		CancelledAt:     tossResp.CanceledAt,
		RawResponse:     string(respBody),
	}, nil
}

// HandleWebhook 웹훅 처리
func (g *TossPaymentsGateway) HandleWebhook(ctx context.Context, payload []byte) (*WebhookResult, error) {
	var webhook TossWebhookPayload
	if err := json.Unmarshal(payload, &webhook); err != nil {
		return nil, ErrInvalidWebhook
	}

	result := &WebhookResult{
		EventType: webhook.EventType,
		PGTID:     webhook.Data.PaymentKey,
		PGOrderID: webhook.Data.OrderID,
		Amount:    webhook.Data.TotalAmount,
		RawData:   string(payload),
	}

	switch webhook.EventType {
	case "PAYMENT_STATUS_CHANGED":
		switch webhook.Data.Status {
		case "DONE":
			result.Status = domain.PaymentStatusPaid
		case "CANCELED":
			result.Status = domain.PaymentStatusCancelled
		case "WAITING_FOR_DEPOSIT":
			result.Status = domain.PaymentStatusReady
			if webhook.Data.VirtualAccount != nil {
				result.VBankName = webhook.Data.VirtualAccount.BankCode
				result.VBankNumber = webhook.Data.VirtualAccount.AccountNumber
				result.VBankHolder = webhook.Data.VirtualAccount.CustomerName
			}
		case "PARTIAL_CANCELED":
			result.Status = domain.PaymentStatusPartialCancelled
		default:
			result.Status = domain.PaymentStatusFailed
		}
	case "DEPOSIT_CALLBACK":
		result.Status = domain.PaymentStatusPaid
	default:
		return nil, ErrInvalidWebhook
	}

	return result, nil
}

// Verify 결제 검증
func (g *TossPaymentsGateway) Verify(ctx context.Context, pgTID string, amount float64) error {
	// 결제 조회 API 호출하여 금액 검증
	queryURL := g.getAPIURL() + fmt.Sprintf("/v1/payments/%s", pgTID)

	httpReq, err := http.NewRequestWithContext(ctx, "GET", queryURL, nil)
	if err != nil {
		return err
	}

	auth := base64.StdEncoding.EncodeToString([]byte(g.config.SecretKey + ":"))
	httpReq.Header.Set("Authorization", "Basic "+auth)

	resp, err := g.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ErrPaymentNotFound
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var tossResp TossPaymentResponse
	if err := json.Unmarshal(respBody, &tossResp); err != nil {
		return err
	}

	if tossResp.TotalAmount != amount {
		return ErrInvalidAmount
	}

	return nil
}

// getAPIURL API URL 반환
func (g *TossPaymentsGateway) getAPIURL() string {
	if g.config.IsSandbox {
		return "https://api.tosspayments.com"
	}
	return "https://api.tosspayments.com"
}

// convertToCompleteResponse 토스 응답을 CompleteResponse로 변환
func (g *TossPaymentsGateway) convertToCompleteResponse(tossResp *TossPaymentResponse, rawResponse string) *CompleteResponse {
	response := &CompleteResponse{
		Success:       tossResp.Status == "DONE",
		PGTID:         tossResp.PaymentKey,
		PGOrderID:     tossResp.OrderID,
		Amount:        tossResp.TotalAmount,
		PaymentMethod: g.convertPaymentMethod(tossResp.Method),
		Status:        g.convertStatus(tossResp.Status),
		RawResponse:   rawResponse,
	}

	// 카드 결제
	if tossResp.Card != nil {
		response.CardCompany = tossResp.Card.IssuerCode
		response.CardNumber = tossResp.Card.Number
		response.CardType = tossResp.Card.CardType
		response.InstallMonth = tossResp.Card.InstallmentPlanMonths
	}

	// 가상계좌
	if tossResp.VirtualAccount != nil {
		response.VBankName = tossResp.VirtualAccount.BankCode
		response.VBankNumber = tossResp.VirtualAccount.AccountNumber
		response.VBankHolder = tossResp.VirtualAccount.CustomerName
		response.VBankDue = tossResp.VirtualAccount.DueDate
	}

	// 수수료 (토스페이먼츠는 별도 조회 필요)
	response.PGFee = tossResp.TotalAmount * 0.029 // 예상 수수료

	return response
}

// convertPaymentMethod 결제 수단 변환
func (g *TossPaymentsGateway) convertPaymentMethod(method string) domain.PaymentMethod {
	switch method {
	case "카드":
		return domain.PaymentMethodCard
	case "계좌이체":
		return domain.PaymentMethodBank
	case "가상계좌":
		return domain.PaymentMethodVirtual
	case "휴대폰":
		return domain.PaymentMethodPhone
	default:
		return domain.PaymentMethodCard
	}
}

// convertStatus 상태 변환
func (g *TossPaymentsGateway) convertStatus(status string) domain.PaymentStatus {
	switch status {
	case "DONE":
		return domain.PaymentStatusPaid
	case "CANCELED":
		return domain.PaymentStatusCancelled
	case "PARTIAL_CANCELED":
		return domain.PaymentStatusPartialCancelled
	case "WAITING_FOR_DEPOSIT":
		return domain.PaymentStatusReady
	default:
		return domain.PaymentStatusFailed
	}
}

// TossPaymentResponse 토스페이먼츠 결제 응답
type TossPaymentResponse struct {
	PaymentKey    string  `json:"paymentKey"`
	OrderID       string  `json:"orderId"`
	OrderName     string  `json:"orderName"`
	Status        string  `json:"status"`
	Method        string  `json:"method"`
	TotalAmount   float64 `json:"totalAmount"`
	BalanceAmount float64 `json:"balanceAmount"`
	ApprovedAt    string  `json:"approvedAt"`
	CanceledAt    string  `json:"canceledAt,omitempty"`

	Card           *TossCardInfo           `json:"card,omitempty"`
	VirtualAccount *TossVirtualAccountInfo `json:"virtualAccount,omitempty"`
	Cancels        []TossCancelInfo        `json:"cancels,omitempty"`
}

// TossCardInfo 카드 정보
type TossCardInfo struct {
	IssuerCode            string `json:"issuerCode"`
	AcquirerCode          string `json:"acquirerCode"`
	Number                string `json:"number"`
	InstallmentPlanMonths int    `json:"installmentPlanMonths"`
	CardType              string `json:"cardType"`
}

// TossVirtualAccountInfo 가상계좌 정보
type TossVirtualAccountInfo struct {
	AccountNumber string `json:"accountNumber"`
	BankCode      string `json:"bankCode"`
	CustomerName  string `json:"customerName"`
	DueDate       string `json:"dueDate"`
}

// TossCancelInfo 취소 정보
type TossCancelInfo struct {
	CancelAmount  float64 `json:"cancelAmount"`
	CancelReason  string  `json:"cancelReason"`
	CanceledAt    string  `json:"canceledAt"`
}

// TossErrorResponse 토스 에러 응답
type TossErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// TossWebhookPayload 토스 웹훅 페이로드
type TossWebhookPayload struct {
	EventType string `json:"eventType"`
	Data      struct {
		PaymentKey     string                  `json:"paymentKey"`
		OrderID        string                  `json:"orderId"`
		Status         string                  `json:"status"`
		TotalAmount    float64                 `json:"totalAmount"`
		VirtualAccount *TossVirtualAccountInfo `json:"virtualAccount,omitempty"`
	} `json:"data"`
}
