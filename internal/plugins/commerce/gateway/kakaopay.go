package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/damoang/angple-backend/internal/plugins/commerce/domain"
)

// KakaoPayConfig 카카오페이 설정
type KakaoPayConfig struct {
	CID           string // 가맹점 코드 (테스트: TC0ONETIME)
	AdminKey      string // Admin 키 (REST API 키)
	SecretKey     string // Secret 키 (v1 API용, 선택)
	IsSandbox     bool   // 테스트 모드
	ApprovalURL   string // 결제 성공 시 리다이렉트 URL
	CancelURL     string // 결제 취소 시 리다이렉트 URL
	FailURL       string // 결제 실패 시 리다이렉트 URL
}

// KakaoPayGateway 카카오페이 게이트웨이 구현
type KakaoPayGateway struct {
	config     *KakaoPayConfig
	httpClient *http.Client
	// 임시 저장소 (실제로는 Redis 등 사용 권장)
	readyStore map[string]*KakaoPayReadyResponse
}

// NewKakaoPayGateway 생성자
func NewKakaoPayGateway(config *KakaoPayConfig) *KakaoPayGateway {
	return &KakaoPayGateway{
		config: config,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		readyStore: make(map[string]*KakaoPayReadyResponse),
	}
}

// Provider PG사 이름 반환
func (g *KakaoPayGateway) Provider() domain.PGProvider {
	return domain.PGProviderKakaoPay
}

// Prepare 결제 준비 (Ready API 호출)
func (g *KakaoPayGateway) Prepare(ctx context.Context, req *PrepareRequest) (*PrepareResponse, error) {
	readyURL := g.getAPIURL() + "/v1/payment/ready"

	pgOrderID := fmt.Sprintf("KAKAO_%s_%d", req.OrderNumber, time.Now().UnixNano()%1000000)

	// Form 데이터 생성
	formData := url.Values{}
	formData.Set("cid", g.config.CID)
	formData.Set("partner_order_id", pgOrderID)
	formData.Set("partner_user_id", fmt.Sprintf("%d", req.OrderID))
	formData.Set("item_name", truncateString(req.ProductName, 100))
	formData.Set("quantity", "1")
	formData.Set("total_amount", fmt.Sprintf("%.0f", req.Amount))
	formData.Set("tax_free_amount", "0")
	formData.Set("approval_url", g.buildCallbackURL(req.ReturnURL, pgOrderID))
	formData.Set("cancel_url", req.CancelURL)
	formData.Set("fail_url", req.CancelURL)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", readyURL, bytes.NewBufferString(formData.Encode()))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Authorization", "KakaoAK "+g.config.AdminKey)
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=utf-8")

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
		var errResp KakaoPayErrorResponse
		if err := json.Unmarshal(respBody, &errResp); err == nil {
			return nil, fmt.Errorf("kakaopay ready failed: %s - %s", errResp.ErrorCode, errResp.ErrorMessage)
		}
		return nil, ErrPaymentFailed
	}

	var kakaoResp KakaoPayReadyResponse
	if err := json.Unmarshal(respBody, &kakaoResp); err != nil {
		return nil, err
	}

	// Ready 응답 저장 (Approve 시 사용)
	g.readyStore[pgOrderID] = &kakaoResp

	return &PrepareResponse{
		PGOrderID:   pgOrderID,
		RedirectURL: kakaoResp.NextRedirectPCURL,
		MobileURL:   kakaoResp.NextRedirectMobileURL,
		AppScheme:   kakaoResp.NextRedirectAppURL,
		ExtraData: map[string]string{
			"tid":                       kakaoResp.TID,
			"next_redirect_pc_url":      kakaoResp.NextRedirectPCURL,
			"next_redirect_mobile_url":  kakaoResp.NextRedirectMobileURL,
			"next_redirect_app_url":     kakaoResp.NextRedirectAppURL,
			"android_app_scheme":        kakaoResp.AndroidAppScheme,
			"ios_app_scheme":            kakaoResp.IOSAppScheme,
		},
	}, nil
}

// Complete 결제 완료 처리 (Approve API 호출)
func (g *KakaoPayGateway) Complete(ctx context.Context, req *CompleteRequest) (*CompleteResponse, error) {
	approveURL := g.getAPIURL() + "/v1/payment/approve"

	// Ready 응답에서 TID 조회
	readyResp, ok := g.readyStore[req.PGOrderID]
	if !ok {
		return nil, ErrPaymentNotFound
	}

	// Form 데이터 생성
	formData := url.Values{}
	formData.Set("cid", g.config.CID)
	formData.Set("tid", readyResp.TID)
	formData.Set("partner_order_id", req.PGOrderID)
	formData.Set("partner_user_id", readyResp.PartnerUserID)
	formData.Set("pg_token", req.PGTID) // pg_token은 카카오페이 결제 완료 후 받는 토큰

	httpReq, err := http.NewRequestWithContext(ctx, "POST", approveURL, bytes.NewBufferString(formData.Encode()))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Authorization", "KakaoAK "+g.config.AdminKey)
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=utf-8")

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
		var errResp KakaoPayErrorResponse
		if err := json.Unmarshal(respBody, &errResp); err == nil {
			return nil, fmt.Errorf("kakaopay approve failed: %s - %s", errResp.ErrorCode, errResp.ErrorMessage)
		}
		return nil, ErrPaymentFailed
	}

	var kakaoResp KakaoPayApproveResponse
	if err := json.Unmarshal(respBody, &kakaoResp); err != nil {
		return nil, err
	}

	// Ready 응답 삭제
	delete(g.readyStore, req.PGOrderID)

	return g.convertToCompleteResponse(&kakaoResp, string(respBody)), nil
}

// Cancel 결제 취소
func (g *KakaoPayGateway) Cancel(ctx context.Context, req *CancelRequest) (*CancelResponse, error) {
	cancelURL := g.getAPIURL() + "/v1/payment/cancel"

	cancelAmount := req.CancelAmount
	if cancelAmount <= 0 {
		cancelAmount = req.TotalAmount
	}

	// Form 데이터 생성
	formData := url.Values{}
	formData.Set("cid", g.config.CID)
	formData.Set("tid", req.PGTID)
	formData.Set("cancel_amount", fmt.Sprintf("%.0f", cancelAmount))
	formData.Set("cancel_tax_free_amount", "0")

	httpReq, err := http.NewRequestWithContext(ctx, "POST", cancelURL, bytes.NewBufferString(formData.Encode()))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Authorization", "KakaoAK "+g.config.AdminKey)
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=utf-8")

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
		var errResp KakaoPayErrorResponse
		if err := json.Unmarshal(respBody, &errResp); err == nil {
			return nil, fmt.Errorf("kakaopay cancel failed: %s - %s", errResp.ErrorCode, errResp.ErrorMessage)
		}
		return nil, ErrCancelNotAllowed
	}

	var kakaoResp KakaoPayCancelResponse
	if err := json.Unmarshal(respBody, &kakaoResp); err != nil {
		return nil, err
	}

	return &CancelResponse{
		Success:          true,
		CancelledAmount:  float64(kakaoResp.CanceledAmount.Total),
		RemainingAmount:  float64(kakaoResp.Amount.Total - kakaoResp.CanceledAmount.Total),
		CancelledAt:      kakaoResp.CanceledAt,
		RawResponse:      string(respBody),
	}, nil
}

// HandleWebhook 웹훅 처리 (카카오페이는 기본 웹훅 미지원, 결제 상태 조회 API 사용)
func (g *KakaoPayGateway) HandleWebhook(ctx context.Context, payload []byte) (*WebhookResult, error) {
	// 카카오페이는 웹훅 대신 결제 완료 후 리다이렉트로 처리
	// 필요시 Order API를 통해 결제 상태 조회 가능
	return nil, ErrInvalidWebhook
}

// Verify 결제 검증 (Order API 호출)
func (g *KakaoPayGateway) Verify(ctx context.Context, pgTID string, amount float64) error {
	orderURL := g.getAPIURL() + "/v1/payment/order"

	// Form 데이터 생성
	formData := url.Values{}
	formData.Set("cid", g.config.CID)
	formData.Set("tid", pgTID)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", orderURL, bytes.NewBufferString(formData.Encode()))
	if err != nil {
		return err
	}

	httpReq.Header.Set("Authorization", "KakaoAK "+g.config.AdminKey)
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=utf-8")

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

	var kakaoResp KakaoPayOrderResponse
	if err := json.Unmarshal(respBody, &kakaoResp); err != nil {
		return err
	}

	if float64(kakaoResp.Amount.Total) != amount {
		return ErrInvalidAmount
	}

	return nil
}

// getAPIURL API URL 반환
func (g *KakaoPayGateway) getAPIURL() string {
	// 카카오페이는 동일 URL (CID로 테스트/실제 구분)
	return "https://kapi.kakao.com"
}

// buildCallbackURL 콜백 URL 생성
func (g *KakaoPayGateway) buildCallbackURL(returnURL string, pgOrderID string) string {
	u, err := url.Parse(returnURL)
	if err != nil {
		return returnURL
	}
	q := u.Query()
	q.Set("pg_order_id", pgOrderID)
	u.RawQuery = q.Encode()
	return u.String()
}

// convertToCompleteResponse 카카오페이 응답을 CompleteResponse로 변환
func (g *KakaoPayGateway) convertToCompleteResponse(kakaoResp *KakaoPayApproveResponse, rawResponse string) *CompleteResponse {
	response := &CompleteResponse{
		Success:       true,
		PGTID:         kakaoResp.TID,
		PGOrderID:     kakaoResp.PartnerOrderID,
		Amount:        float64(kakaoResp.Amount.Total),
		PaymentMethod: domain.PaymentMethodCard, // 카카오페이는 기본 카드
		Status:        domain.PaymentStatusPaid,
		RawResponse:   rawResponse,
	}

	// 카드 정보
	if kakaoResp.CardInfo != nil {
		response.CardCompany = kakaoResp.CardInfo.KakaoPayPurchaseCorp
		response.CardNumber = ""
		response.CardType = kakaoResp.CardInfo.CardType
		response.InstallMonth = kakaoResp.CardInfo.InstallMonth
	}

	// 수수료 (카카오페이 기본 수수료율)
	response.PGFee = float64(kakaoResp.Amount.Total) * 0.033

	return response
}

// truncateString 문자열 길이 제한
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

// ========================================
// 카카오페이 API 응답 구조체
// ========================================

// KakaoPayReadyResponse Ready API 응답
type KakaoPayReadyResponse struct {
	TID                    string `json:"tid"`
	NextRedirectAppURL     string `json:"next_redirect_app_url"`
	NextRedirectMobileURL  string `json:"next_redirect_mobile_url"`
	NextRedirectPCURL      string `json:"next_redirect_pc_url"`
	AndroidAppScheme       string `json:"android_app_scheme"`
	IOSAppScheme           string `json:"ios_app_scheme"`
	CreatedAt              string `json:"created_at"`
	PartnerUserID          string `json:"-"` // 저장용
}

// KakaoPayApproveResponse Approve API 응답
type KakaoPayApproveResponse struct {
	AID             string            `json:"aid"`
	TID             string            `json:"tid"`
	CID             string            `json:"cid"`
	PartnerOrderID  string            `json:"partner_order_id"`
	PartnerUserID   string            `json:"partner_user_id"`
	PaymentMethodType string          `json:"payment_method_type"`
	Amount          KakaoPayAmount    `json:"amount"`
	CardInfo        *KakaoPayCardInfo `json:"card_info,omitempty"`
	ItemName        string            `json:"item_name"`
	Quantity        int               `json:"quantity"`
	CreatedAt       string            `json:"created_at"`
	ApprovedAt      string            `json:"approved_at"`
}

// KakaoPayAmount 금액 정보
type KakaoPayAmount struct {
	Total    int `json:"total"`
	TaxFree  int `json:"tax_free"`
	VAT      int `json:"vat"`
	Point    int `json:"point"`
	Discount int `json:"discount"`
}

// KakaoPayCardInfo 카드 정보
type KakaoPayCardInfo struct {
	KakaoPayPurchaseCorp      string `json:"kakaopay_purchase_corp"`
	KakaoPayPurchaseCorpCode  string `json:"kakaopay_purchase_corp_code"`
	KakaoPayIssuerCorp        string `json:"kakaopay_issuer_corp"`
	KakaoPayIssuerCorpCode    string `json:"kakaopay_issuer_corp_code"`
	Bin                       string `json:"bin"`
	CardType                  string `json:"card_type"`
	InstallMonth              int    `json:"install_month"`
	ApprovedID                string `json:"approved_id"`
	CardMid                   string `json:"card_mid"`
	InterestFreeInstall       string `json:"interest_free_install"`
	CardItemCode              string `json:"card_item_code"`
}

// KakaoPayCancelResponse Cancel API 응답
type KakaoPayCancelResponse struct {
	AID            string         `json:"aid"`
	TID            string         `json:"tid"`
	CID            string         `json:"cid"`
	Status         string         `json:"status"`
	PartnerOrderID string         `json:"partner_order_id"`
	PartnerUserID  string         `json:"partner_user_id"`
	Amount         KakaoPayAmount `json:"amount"`
	CanceledAmount KakaoPayAmount `json:"canceled_amount"`
	CancelAvailableAmount KakaoPayAmount `json:"cancel_available_amount"`
	CanceledAt     string         `json:"canceled_at"`
}

// KakaoPayOrderResponse Order API 응답
type KakaoPayOrderResponse struct {
	TID            string         `json:"tid"`
	CID            string         `json:"cid"`
	Status         string         `json:"status"`
	PartnerOrderID string         `json:"partner_order_id"`
	PartnerUserID  string         `json:"partner_user_id"`
	Amount         KakaoPayAmount `json:"amount"`
	CanceledAmount KakaoPayAmount `json:"canceled_amount"`
	CancelAvailableAmount KakaoPayAmount `json:"cancel_available_amount"`
	ItemName       string         `json:"item_name"`
	CreatedAt      string         `json:"created_at"`
	ApprovedAt     string         `json:"approved_at"`
	CanceledAt     string         `json:"canceled_at,omitempty"`
}

// KakaoPayErrorResponse 에러 응답
type KakaoPayErrorResponse struct {
	ErrorCode    string `json:"error_code"`
	ErrorMessage string `json:"error_message"`
	Extras       struct {
		MethodResultCode    string `json:"method_result_code,omitempty"`
		MethodResultMessage string `json:"method_result_message,omitempty"`
	} `json:"extras,omitempty"`
}
