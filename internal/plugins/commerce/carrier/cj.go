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

// CJCarrier CJ대한통운 배송 추적
type CJCarrier struct {
	httpClient *http.Client
	apiKey     string // SweetTracker API 키 (선택)
}

// NewCJCarrier 생성자
func NewCJCarrier(apiKey string) *CJCarrier {
	return &CJCarrier{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		apiKey: apiKey,
	}
}

// Code 배송사 코드 반환
func (c *CJCarrier) Code() domain.ShippingCarrierCode {
	return domain.CarrierCJ
}

// ValidateTrackingNumber 송장번호 유효성 검증
func (c *CJCarrier) ValidateTrackingNumber(trackingNumber string) bool {
	// CJ대한통운 송장번호: 10~12자리 숫자
	re := regexp.MustCompile(`^\d{10,12}$`)
	return re.MatchString(trackingNumber)
}

// Track 배송 추적 (SweetTracker API 또는 직접 스크래핑)
func (c *CJCarrier) Track(ctx context.Context, trackingNumber string) (*domain.TrackingInfo, error) {
	// SweetTracker API 사용 (무료 계정: 일 500건)
	if c.apiKey != "" {
		return c.trackWithSweetTracker(ctx, trackingNumber)
	}

	// 기본 Mock 데이터 반환 (실제 환경에서는 직접 API 연동 필요)
	return c.mockTrackingInfo(trackingNumber)
}

// trackWithSweetTracker SweetTracker API 호출
func (c *CJCarrier) trackWithSweetTracker(ctx context.Context, trackingNumber string) (*domain.TrackingInfo, error) {
	url := fmt.Sprintf("http://info.sweettracker.co.kr/api/v1/trackingInfo?t_key=%s&t_code=04&t_invoice=%s",
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
func (c *CJCarrier) convertSweetTrackerResponse(resp *SweetTrackerResponse, trackingNumber string) *domain.TrackingInfo {
	info := &domain.TrackingInfo{
		Carrier:        domain.GetShippingCarrier("cj"),
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
func (c *CJCarrier) convertStatus(kind string) domain.ShippingStatus {
	switch kind {
	case "간선하차", "배송출발", "배송중":
		return domain.ShippingStatusInTransit
	case "배달출발":
		return domain.ShippingStatusOutForDelivery
	case "배달완료":
		return domain.ShippingStatusDelivered
	case "집화처리":
		return domain.ShippingStatusPickedUp
	default:
		return domain.ShippingStatusInTransit
	}
}

// mockTrackingInfo Mock 데이터 (개발/테스트용)
func (c *CJCarrier) mockTrackingInfo(trackingNumber string) (*domain.TrackingInfo, error) {
	now := time.Now()

	return &domain.TrackingInfo{
		Carrier:        domain.GetShippingCarrier("cj"),
		TrackingNumber: trackingNumber,
		Status:         domain.ShippingStatusInTransit,
		StatusText:     "배송 중",
		Events: []*domain.TrackingEvent{
			{
				Time:        now.Add(-48 * time.Hour),
				Location:    "서울 강남 영업소",
				Status:      "집화처리",
				Description: "물품을 인수하였습니다.",
			},
			{
				Time:        now.Add(-36 * time.Hour),
				Location:    "서울 HUB",
				Status:      "간선상차",
				Description: "물품이 간선 상차되었습니다.",
			},
			{
				Time:        now.Add(-24 * time.Hour),
				Location:    "부산 HUB",
				Status:      "간선하차",
				Description: "물품이 간선 하차되었습니다.",
			},
			{
				Time:        now.Add(-12 * time.Hour),
				Location:    "부산 해운대 영업소",
				Status:      "배송출발",
				Description: "배송을 시작합니다.",
			},
		},
	}, nil
}

// SweetTracker API 응답 구조체
type SweetTrackerResponse struct {
	Code    string `json:"code"`
	Message string `json:"msg"`
	State   struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	} `json:"state"`
	SenderName      string `json:"senderName"`
	ReceiverName    string `json:"receiverName"`
	ReceiverAddress string `json:"receiverAddr"`
	ItemName        string `json:"itemName"`
	LastDetail      struct {
		Kind       string `json:"kind"`
		Level      string `json:"level"`
		TimeString string `json:"timeString"`
		Where      string `json:"where"`
	} `json:"lastDetail"`
	TrackingDetails []struct {
		Kind       string `json:"kind"`
		Level      string `json:"level"`
		TimeString string `json:"timeString"`
		Where      string `json:"where"`
	} `json:"trackingDetails"`
}
