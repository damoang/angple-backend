package gateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/damoang/angple-backend/internal/plugins/commerce/domain"
)

// InicisConfig KG이니시스 설정
type InicisConfig struct {
	MerchantID string // 상점 ID (MID)
	SignKey    string // 서명키
	APIKey     string // API 키
	APISecret  string // API 시크릿
	IsSandbox  bool   // 테스트 모드
}

// InicisGateway KG이니시스 게이트웨이 구현
type InicisGateway struct {
	config *InicisConfig
}

// NewInicisGateway 생성자
func NewInicisGateway(config *InicisConfig) *InicisGateway {
	return &InicisGateway{config: config}
}

// Provider PG사 이름 반환
func (g *InicisGateway) Provider() domain.PGProvider {
	return domain.PGProviderInicis
}

// Prepare 결제 준비
func (g *InicisGateway) Prepare(ctx context.Context, req *PrepareRequest) (*PrepareResponse, error) {
	// 주문번호 기반 PG 주문 ID 생성
	pgOrderID := fmt.Sprintf("INI_%s_%d", req.OrderNumber, time.Now().UnixNano()%1000000)
	timestamp := time.Now().Format("20060102150405")

	// 서명 생성
	signature := g.generateSignature(pgOrderID, req.Amount, timestamp)

	// 결제창 URL 구성
	baseURL := g.getBaseURL()
	params := url.Values{}
	params.Set("gopaymethod", g.convertPaymentMethod(req.PaymentMethod))
	params.Set("mid", g.config.MerchantID)
	params.Set("oid", pgOrderID)
	params.Set("price", fmt.Sprintf("%.0f", req.Amount))
	params.Set("goodname", req.ProductName)
	params.Set("buyername", req.BuyerName)
	params.Set("buyertel", req.BuyerPhone)
	params.Set("buyeremail", req.BuyerEmail)
	params.Set("returnUrl", req.ReturnURL)
	params.Set("closeUrl", req.CancelURL)
	params.Set("timestamp", timestamp)
	params.Set("signature", signature)
	params.Set("mKey", g.generateMKey())

	redirectURL := fmt.Sprintf("%s/stdpay/ini_stdpay.php?%s", baseURL, params.Encode())
	mobileURL := fmt.Sprintf("%s/mobile/INIpayMobile.php?%s", baseURL, params.Encode())

	return &PrepareResponse{
		PGOrderID:   pgOrderID,
		RedirectURL: redirectURL,
		MobileURL:   mobileURL,
		MerchantID:  g.config.MerchantID,
		Signature:   signature,
		Timestamp:   timestamp,
		ExtraData: map[string]string{
			"mKey":       g.generateMKey(),
			"gopaymethod": g.convertPaymentMethod(req.PaymentMethod),
		},
	}, nil
}

// Complete 결제 완료 처리
func (g *InicisGateway) Complete(ctx context.Context, req *CompleteRequest) (*CompleteResponse, error) {
	// 금액 검증
	if err := g.Verify(ctx, req.PGTID, req.Amount); err != nil {
		return nil, err
	}

	// 실제 환경에서는 이니시스 승인 API 호출
	// 여기서는 시뮬레이션
	response := &CompleteResponse{
		Success:       true,
		PGTID:         req.PGTID,
		PGOrderID:     req.PGOrderID,
		Amount:        req.Amount,
		PaymentMethod: domain.PaymentMethodCard,
		Status:        domain.PaymentStatusPaid,
		CardCompany:   "신한카드",
		CardNumber:    "****-****-****-1234",
		CardType:      "credit",
		InstallMonth:  0,
		PGFee:         req.Amount * 0.033, // 3.3% 수수료
		RawResponse:   req.RawResponse,
	}

	return response, nil
}

// Cancel 결제 취소
func (g *InicisGateway) Cancel(ctx context.Context, req *CancelRequest) (*CancelResponse, error) {
	// 실제 환경에서는 이니시스 취소 API 호출
	// 여기서는 시뮬레이션
	cancelledAt := time.Now().Format(time.RFC3339)

	return &CancelResponse{
		Success:         true,
		CancelledAmount: req.CancelAmount,
		RemainingAmount: req.TotalAmount - req.CancelAmount,
		CancelledAt:     cancelledAt,
		RawResponse:     fmt.Sprintf(`{"resultCode":"00","resultMsg":"취소성공","cancelAmount":%.0f}`, req.CancelAmount),
	}, nil
}

// HandleWebhook 웹훅 처리
func (g *InicisGateway) HandleWebhook(ctx context.Context, payload []byte) (*WebhookResult, error) {
	var data map[string]interface{}
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, ErrInvalidWebhook
	}

	// 웹훅 서명 검증
	// 실제 환경에서는 이니시스 웹훅 서명 검증 필요

	eventType, _ := data["type"].(string)
	pgTID, _ := data["tid"].(string)
	pgOrderID, _ := data["oid"].(string)
	amount, _ := data["price"].(float64)

	result := &WebhookResult{
		EventType: eventType,
		PGTID:     pgTID,
		PGOrderID: pgOrderID,
		Amount:    amount,
		RawData:   string(payload),
	}

	switch eventType {
	case "paid":
		result.Status = domain.PaymentStatusPaid
	case "cancelled":
		result.Status = domain.PaymentStatusCancelled
	case "vbank_issued":
		result.Status = domain.PaymentStatusReady
		result.VBankName, _ = data["vbankName"].(string)
		result.VBankNumber, _ = data["vbankNum"].(string)
		result.VBankHolder, _ = data["vbankHolder"].(string)
	case "vbank_paid":
		result.Status = domain.PaymentStatusPaid
	default:
		return nil, ErrInvalidWebhook
	}

	return result, nil
}

// Verify 결제 검증
func (g *InicisGateway) Verify(ctx context.Context, pgTID string, amount float64) error {
	// 실제 환경에서는 이니시스 조회 API로 금액 검증
	// 여기서는 기본 검증만
	if amount <= 0 {
		return ErrInvalidAmount
	}
	return nil
}

// generateSignature 서명 생성
func (g *InicisGateway) generateSignature(orderID string, amount float64, timestamp string) string {
	data := fmt.Sprintf("%s%s%.0f%s", g.config.MerchantID, orderID, amount, timestamp)
	hash := sha256.Sum256([]byte(data + g.config.SignKey))
	return hex.EncodeToString(hash[:])
}

// generateMKey mKey 생성
func (g *InicisGateway) generateMKey() string {
	hash := sha256.Sum256([]byte(g.config.SignKey))
	return hex.EncodeToString(hash[:])
}

// getBaseURL 기본 URL 반환
func (g *InicisGateway) getBaseURL() string {
	if g.config.IsSandbox {
		return "https://stgstdpay.inicis.com"
	}
	return "https://stdpay.inicis.com"
}

// convertPaymentMethod 결제 수단 변환
func (g *InicisGateway) convertPaymentMethod(method domain.PaymentMethod) string {
	switch method {
	case domain.PaymentMethodCard:
		return "Card"
	case domain.PaymentMethodBank:
		return "DirectBank"
	case domain.PaymentMethodVirtual:
		return "VBank"
	case domain.PaymentMethodPhone:
		return "HPP"
	default:
		return "Card"
	}
}

// ParseCallback 콜백 파라미터 파싱
func (g *InicisGateway) ParseCallback(params map[string]string) (*CompleteRequest, error) {
	resultCode := params["resultCode"]
	if resultCode != "0000" {
		return nil, fmt.Errorf("payment failed: %s - %s", resultCode, params["resultMsg"])
	}

	amount := 0.0
	if amtStr := params["TotPrice"]; amtStr != "" {
		fmt.Sscanf(amtStr, "%f", &amount)
	}

	return &CompleteRequest{
		PGTID:       params["tid"],
		PGOrderID:   params["MOID"],
		Amount:      amount,
		RawResponse: formatParams(params),
	}, nil
}

// formatParams 파라미터를 JSON 문자열로 변환
func formatParams(params map[string]string) string {
	data, _ := json.Marshal(params)
	return string(data)
}

// InicisCardInfo 카드 정보 파싱
type InicisCardInfo struct {
	CardCode    string
	CardName    string
	CardNumber  string
	CardType    string // 0: 신용, 1: 체크
	InstallMonth int
}

// ParseCardInfo 카드 정보 파싱
func (g *InicisGateway) ParseCardInfo(params map[string]string) *InicisCardInfo {
	installMonth := 0
	if quota := params["CARD_Quota"]; quota != "" {
		fmt.Sscanf(quota, "%d", &installMonth)
	}

	cardType := "credit"
	if params["CARD_CheckFlag"] == "1" {
		cardType = "check"
	}

	return &InicisCardInfo{
		CardCode:     params["CARD_Code"],
		CardName:     g.getCardName(params["CARD_Code"]),
		CardNumber:   maskCardNumber(params["CARD_Num"]),
		CardType:     cardType,
		InstallMonth: installMonth,
	}
}

// getCardName 카드사 이름 반환
func (g *InicisGateway) getCardName(code string) string {
	cardNames := map[string]string{
		"01": "외환카드", "02": "롯데카드", "03": "현대카드",
		"04": "삼성카드", "06": "신한카드", "07": "현대카드",
		"08": "롯데카드", "11": "BC카드", "12": "삼성카드",
		"13": "광주은행", "14": "전북은행", "15": "제주은행",
		"21": "국민카드", "22": "농협카드", "23": "우리카드",
		"24": "씨티카드", "25": "KDB산업은행", "26": "수협카드",
	}
	if name, ok := cardNames[code]; ok {
		return name
	}
	return "카드"
}

// maskCardNumber 카드번호 마스킹
func maskCardNumber(cardNum string) string {
	if len(cardNum) < 8 {
		return cardNum
	}
	// 앞 4자리, 뒤 4자리만 표시
	cardNum = strings.ReplaceAll(cardNum, "-", "")
	if len(cardNum) >= 16 {
		return cardNum[:4] + "-****-****-" + cardNum[12:]
	}
	return "****-****-****-" + cardNum[len(cardNum)-4:]
}
