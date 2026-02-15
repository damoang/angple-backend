package carrier

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"

	"github.com/damoang/angple-backend/internal/plugins/commerce/domain"
)

// LotteCarrier 롯데택배 배송 추적
type LotteCarrier struct {
	httpClient *http.Client
	apiKey     string // SweetTracker API 키 (선택)
}

// NewLotteCarrier 생성자
func NewLotteCarrier(apiKey string) *LotteCarrier {
	return &LotteCarrier{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		apiKey: apiKey,
	}
}

// Code 배송사 코드 반환
func (c *LotteCarrier) Code() domain.ShippingCarrierCode {
	return domain.CarrierLotte
}

// ValidateTrackingNumber 송장번호 유효성 검증
func (c *LotteCarrier) ValidateTrackingNumber(trackingNumber string) bool {
	// 롯데택배 송장번호: 10~12자리 숫자
	re := regexp.MustCompile(`^\d{10,12}$`)
	return re.MatchString(trackingNumber)
}

// Track 배송 추적
func (c *LotteCarrier) Track(ctx context.Context, trackingNumber string) (*domain.TrackingInfo, error) {
	// SweetTracker API 사용
	if c.apiKey != "" {
		return c.trackWithSweetTracker(ctx, trackingNumber)
	}

	// 기본 Mock 데이터 반환
	return c.mockTrackingInfo(trackingNumber)
}

// trackWithSweetTracker SweetTracker API 호출
func (c *LotteCarrier) trackWithSweetTracker(ctx context.Context, trackingNumber string) (*domain.TrackingInfo, error) {
	// 롯데택배 코드: 08
	url := fmt.Sprintf("http://info.sweettracker.co.kr/api/v1/trackingInfo?t_key=%s&t_code=08&t_invoice=%s",
		c.apiKey, trackingNumber)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, ErrTrackingAPIFailed
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var sweetResp SweetTrackerResponse
	if err := json.Unmarshal(body, &sweetResp); err != nil {
		return nil, err
	}

	if sweetResp.Code != "0" {
		return nil, ErrTrackingNotFound
	}

	return c.convertSweetTrackerResponse(&sweetResp, trackingNumber), nil
}

// convertSweetTrackerResponse SweetTracker 응답 변환
func (c *LotteCarrier) convertSweetTrackerResponse(resp *SweetTrackerResponse, trackingNumber string) *domain.TrackingInfo {
	info := &domain.TrackingInfo{
		Carrier:        domain.GetShippingCarrier("lotte"),
		TrackingNumber: trackingNumber,
		Status:         c.convertStatus(resp.LastDetail.Kind),
		StatusText:     resp.LastDetail.Level,
		Events:         make([]*domain.TrackingEvent, 0, len(resp.TrackingDetails)),
	}

	// 이벤트 변환
	for _, detail := range resp.TrackingDetails {
		t, _ := time.Parse("2006-01-02 15:04:05", detail.TimeString)
		info.Events = append(info.Events, &domain.TrackingEvent{
			Time:        t,
			Location:    detail.Where,
			Status:      detail.Level,
			Description: detail.Kind,
		})
	}

	// 배송 완료 시간
	if info.Status == domain.ShippingStatusDelivered && len(info.Events) > 0 {
		deliveredAt := info.Events[len(info.Events)-1].Time
		info.DeliveredAt = &deliveredAt
	}

	return info
}

// convertStatus 상태 변환
func (c *LotteCarrier) convertStatus(kind string) domain.ShippingStatus {
	switch kind {
	case "터미널입고", "터미널출고", "배송중":
		return domain.ShippingStatusInTransit
	case "배달출발":
		return domain.ShippingStatusOutForDelivery
	case "배달완료":
		return domain.ShippingStatusDelivered
	case "집화":
		return domain.ShippingStatusPickedUp
	default:
		return domain.ShippingStatusInTransit
	}
}

// mockTrackingInfo Mock 데이터 (개발/테스트용)
func (c *LotteCarrier) mockTrackingInfo(trackingNumber string) (*domain.TrackingInfo, error) {
	now := time.Now()

	return &domain.TrackingInfo{
		Carrier:        domain.GetShippingCarrier("lotte"),
		TrackingNumber: trackingNumber,
		Status:         domain.ShippingStatusInTransit,
		StatusText:     "배송 중",
		Events: []*domain.TrackingEvent{
			{
				Time:        now.Add(-48 * time.Hour),
				Location:    "서울 강남 집배점",
				Status:      "집화",
				Description: "집화처리 되었습니다.",
			},
			{
				Time:        now.Add(-36 * time.Hour),
				Location:    "서울 터미널",
				Status:      "터미널입고",
				Description: "터미널에 입고되었습니다.",
			},
			{
				Time:        now.Add(-24 * time.Hour),
				Location:    "서울 터미널",
				Status:      "터미널출고",
				Description: "터미널에서 출고되었습니다.",
			},
			{
				Time:        now.Add(-12 * time.Hour),
				Location:    "부산 해운대 집배점",
				Status:      "배송중",
				Description: "배송 중입니다.",
			},
		},
	}, nil
}
