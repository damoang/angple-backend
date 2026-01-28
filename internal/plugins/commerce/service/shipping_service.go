package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/damoang/angple-backend/internal/plugins/commerce/carrier"
	"github.com/damoang/angple-backend/internal/plugins/commerce/domain"
	"github.com/damoang/angple-backend/internal/plugins/commerce/repository"
)

// 배송 에러 정의
var (
	ErrShippingNotAllowed    = errors.New("shipping not allowed for this order")
	ErrShippingAlreadySet    = errors.New("shipping info already set")
	ErrInvalidShippingStatus = errors.New("invalid shipping status for this action")
	ErrCarrierNotSupported   = errors.New("carrier not supported")
)

// ShippingService 배송 서비스 인터페이스
type ShippingService interface {
	// 송장번호 등록
	RegisterShipping(ctx context.Context, sellerID, orderID uint64, req *domain.RegisterShippingRequest) error

	// 배송 추적
	TrackShipping(ctx context.Context, userID, orderID uint64) (*domain.TrackingResponse, error)

	// 배송사 목록
	GetCarriers() *domain.ShippingCarrierListResponse

	// 배송 완료 처리
	MarkDelivered(ctx context.Context, sellerID, orderID uint64) error

	// 배송 상태 업데이트 (웹훅 또는 배치)
	UpdateShippingStatus(ctx context.Context, orderID uint64) error
}

// shippingService 구현체
type shippingService struct {
	orderRepo      repository.OrderRepository
	carrierManager *carrier.CarrierManager
}

// NewShippingService 생성자
func NewShippingService(orderRepo repository.OrderRepository, carrierManager *carrier.CarrierManager) ShippingService {
	return &shippingService{
		orderRepo:      orderRepo,
		carrierManager: carrierManager,
	}
}

// RegisterShipping 송장번호 등록
func (s *shippingService) RegisterShipping(ctx context.Context, sellerID, orderID uint64, req *domain.RegisterShippingRequest) error {
	// 주문 조회
	order, err := s.orderRepo.FindByID(orderID)
	if err != nil {
		return ErrOrderNotFound
	}

	// 판매자 권한 확인 (주문에 해당 판매자의 상품이 있는지)
	hasSellerItem := false
	for _, item := range order.Items {
		if item.SellerID == sellerID {
			hasSellerItem = true
			break
		}
	}
	if !hasSellerItem {
		return ErrOrderForbidden
	}

	// 실물 상품 주문인지 확인
	if !order.HasPhysicalProduct() {
		return ErrShippingNotAllowed
	}

	// 이미 송장번호가 등록되어 있는지 확인
	if order.TrackingNumber != "" {
		return ErrShippingAlreadySet
	}

	// 주문 상태 확인 (paid 또는 processing 상태여야 함)
	if order.Status != domain.OrderStatusPaid && order.Status != domain.OrderStatusProcessing {
		return ErrInvalidShippingStatus
	}

	// 배송사 유효성 확인
	carrierInfo := domain.GetShippingCarrier(req.Carrier)
	if carrierInfo == nil {
		return ErrCarrierNotSupported
	}

	// 송장번호 유효성 검증 (선택적)
	if s.carrierManager != nil {
		c, err := s.carrierManager.Get(domain.ShippingCarrierCode(req.Carrier))
		if err == nil && !c.ValidateTrackingNumber(req.TrackingNumber) {
			return carrier.ErrInvalidTrackingNo
		}
	}

	// 배송 정보 업데이트
	now := time.Now()
	order.ShippingCarrier = req.Carrier
	order.TrackingNumber = req.TrackingNumber
	order.ShippedAt = &now
	order.Status = domain.OrderStatusShipped

	if err := s.orderRepo.Update(orderID, order); err != nil {
		return err
	}

	return nil
}

// TrackShipping 배송 추적
func (s *shippingService) TrackShipping(ctx context.Context, userID, orderID uint64) (*domain.TrackingResponse, error) {
	// 주문 조회
	order, err := s.orderRepo.FindByID(orderID)
	if err != nil {
		return nil, ErrOrderNotFound
	}

	// 사용자 권한 확인
	if order.UserID != userID {
		return nil, ErrOrderForbidden
	}

	// 송장번호 확인
	if order.TrackingNumber == "" {
		return nil, errors.New("tracking number not set")
	}

	// 배송사 정보
	carrierInfo := domain.GetShippingCarrier(order.ShippingCarrier)
	if carrierInfo == nil {
		return nil, ErrCarrierNotSupported
	}

	// 배송 추적 URL 생성
	trackingURL := fmt.Sprintf(carrierInfo.TrackingURL, order.TrackingNumber)

	response := &domain.TrackingResponse{
		OrderID:     order.ID,
		OrderNumber: order.OrderNumber,
		TrackingURL: trackingURL,
	}

	// 실시간 배송 추적 (CarrierManager가 설정된 경우)
	if s.carrierManager != nil {
		trackingInfo, err := s.carrierManager.Track(ctx, order.ShippingCarrier, order.TrackingNumber)
		if err == nil {
			response.TrackingInfo = trackingInfo
		}
	} else {
		// CarrierManager 없으면 기본 정보만 반환
		response.TrackingInfo = &domain.TrackingInfo{
			Carrier:        carrierInfo,
			TrackingNumber: order.TrackingNumber,
			Status:         s.convertOrderStatusToShippingStatus(order.Status),
			StatusText:     s.getStatusText(order.Status),
			DeliveredAt:    order.DeliveredAt,
		}
	}

	return response, nil
}

// GetCarriers 배송사 목록
func (s *shippingService) GetCarriers() *domain.ShippingCarrierListResponse {
	return &domain.ShippingCarrierListResponse{
		Carriers: domain.GetAllShippingCarriers(),
	}
}

// MarkDelivered 배송 완료 처리
func (s *shippingService) MarkDelivered(ctx context.Context, sellerID, orderID uint64) error {
	// 주문 조회
	order, err := s.orderRepo.FindByID(orderID)
	if err != nil {
		return ErrOrderNotFound
	}

	// 판매자 권한 확인
	hasSellerItem := false
	for _, item := range order.Items {
		if item.SellerID == sellerID {
			hasSellerItem = true
			break
		}
	}
	if !hasSellerItem {
		return ErrOrderForbidden
	}

	// 주문 상태 확인 (shipped 상태여야 함)
	if order.Status != domain.OrderStatusShipped {
		return ErrInvalidShippingStatus
	}

	// 배송 완료 처리
	now := time.Now()
	order.DeliveredAt = &now
	order.Status = domain.OrderStatusDelivered

	if err := s.orderRepo.Update(orderID, order); err != nil {
		return err
	}

	return nil
}

// UpdateShippingStatus 배송 상태 업데이트 (웹훅 또는 배치)
func (s *shippingService) UpdateShippingStatus(ctx context.Context, orderID uint64) error {
	// 주문 조회
	order, err := s.orderRepo.FindByID(orderID)
	if err != nil {
		return ErrOrderNotFound
	}

	// 송장번호 없으면 스킵
	if order.TrackingNumber == "" {
		return nil
	}

	// 이미 배송 완료면 스킵
	if order.Status == domain.OrderStatusDelivered || order.Status == domain.OrderStatusCompleted {
		return nil
	}

	// CarrierManager가 없으면 스킵
	if s.carrierManager == nil {
		return nil
	}

	// 배송 추적
	trackingInfo, err := s.carrierManager.Track(ctx, order.ShippingCarrier, order.TrackingNumber)
	if err != nil {
		return err
	}

	// 배송 완료 시 상태 업데이트
	if trackingInfo.Status == domain.ShippingStatusDelivered {
		now := time.Now()
		if trackingInfo.DeliveredAt != nil {
			order.DeliveredAt = trackingInfo.DeliveredAt
		} else {
			order.DeliveredAt = &now
		}
		order.Status = domain.OrderStatusDelivered

		if err := s.orderRepo.Update(orderID, order); err != nil {
			return err
		}
	}

	return nil
}

// convertOrderStatusToShippingStatus 주문 상태를 배송 상태로 변환
func (s *shippingService) convertOrderStatusToShippingStatus(status domain.OrderStatus) domain.ShippingStatus {
	switch status {
	case domain.OrderStatusPaid, domain.OrderStatusProcessing:
		return domain.ShippingStatusPending
	case domain.OrderStatusShipped:
		return domain.ShippingStatusInTransit
	case domain.OrderStatusDelivered:
		return domain.ShippingStatusDelivered
	default:
		return domain.ShippingStatusPending
	}
}

// getStatusText 상태 텍스트
func (s *shippingService) getStatusText(status domain.OrderStatus) string {
	switch status {
	case domain.OrderStatusPaid:
		return "결제 완료 (배송 준비 중)"
	case domain.OrderStatusProcessing:
		return "배송 준비 중"
	case domain.OrderStatusShipped:
		return "배송 중"
	case domain.OrderStatusDelivered:
		return "배송 완료"
	default:
		return "알 수 없음"
	}
}
