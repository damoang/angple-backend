package carrier

import (
	"context"
	"errors"
	"sync"

	"github.com/damoang/angple-backend/internal/plugins/commerce/domain"
)

// 배송 추적 에러
var (
	ErrCarrierNotFound   = errors.New("carrier not found")
	ErrTrackingNotFound  = errors.New("tracking info not found")
	ErrInvalidTrackingNo = errors.New("invalid tracking number")
	ErrTrackingAPIFailed = errors.New("tracking API failed")
)

// ShippingCarrier 배송 추적 인터페이스
type ShippingCarrier interface {
	// Code 배송사 코드 반환
	Code() domain.ShippingCarrierCode

	// Track 배송 추적
	Track(ctx context.Context, trackingNumber string) (*domain.TrackingInfo, error)

	// ValidateTrackingNumber 송장번호 유효성 검증
	ValidateTrackingNumber(trackingNumber string) bool
}

// CarrierManager 배송사 관리자
type CarrierManager struct {
	carriers map[domain.ShippingCarrierCode]ShippingCarrier
	mu       sync.RWMutex
}

// NewCarrierManager 배송사 관리자 생성
func NewCarrierManager() *CarrierManager {
	return &CarrierManager{
		carriers: make(map[domain.ShippingCarrierCode]ShippingCarrier),
	}
}

// Register 배송사 등록
func (m *CarrierManager) Register(carrier ShippingCarrier) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.carriers[carrier.Code()] = carrier
}

// Get 배송사 조회
func (m *CarrierManager) Get(code domain.ShippingCarrierCode) (ShippingCarrier, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	carrier, ok := m.carriers[code]
	if !ok {
		return nil, ErrCarrierNotFound
	}
	return carrier, nil
}

// Track 배송 추적
func (m *CarrierManager) Track(ctx context.Context, carrierCode string, trackingNumber string) (*domain.TrackingInfo, error) {
	carrier, err := m.Get(domain.ShippingCarrierCode(carrierCode))
	if err != nil {
		return nil, err
	}

	if !carrier.ValidateTrackingNumber(trackingNumber) {
		return nil, ErrInvalidTrackingNo
	}

	return carrier.Track(ctx, trackingNumber)
}

// List 등록된 배송사 목록
func (m *CarrierManager) List() []domain.ShippingCarrierCode {
	m.mu.RLock()
	defer m.mu.RUnlock()

	codes := make([]domain.ShippingCarrierCode, 0, len(m.carriers))
	for code := range m.carriers {
		codes = append(codes, code)
	}
	return codes
}
