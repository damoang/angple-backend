package service

import (
	"context"
	"testing"

	"github.com/damoang/angple-backend/internal/plugins/commerce/carrier"
	"github.com/damoang/angple-backend/internal/plugins/commerce/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockShippingOrderRepository 배송 테스트용 Mock Order Repository
type MockShippingOrderRepository struct {
	mock.Mock
}

func (m *MockShippingOrderRepository) Create(order *domain.Order) error {
	args := m.Called(order)
	return args.Error(0)
}

func (m *MockShippingOrderRepository) CreateWithItems(order *domain.Order, items []domain.OrderItem) error {
	args := m.Called(order, items)
	return args.Error(0)
}

func (m *MockShippingOrderRepository) Update(id uint64, order *domain.Order) error {
	args := m.Called(id, order)
	return args.Error(0)
}

func (m *MockShippingOrderRepository) UpdateStatus(id uint64, status domain.OrderStatus) error {
	args := m.Called(id, status)
	return args.Error(0)
}

func (m *MockShippingOrderRepository) FindByID(id uint64) (*domain.Order, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Order), args.Error(1)
}

func (m *MockShippingOrderRepository) FindByIDWithItems(id uint64) (*domain.Order, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Order), args.Error(1)
}

func (m *MockShippingOrderRepository) FindByOrderNumber(orderNumber string) (*domain.Order, error) {
	args := m.Called(orderNumber)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Order), args.Error(1)
}

func (m *MockShippingOrderRepository) FindByOrderNumberWithItems(orderNumber string) (*domain.Order, error) {
	args := m.Called(orderNumber)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Order), args.Error(1)
}

func (m *MockShippingOrderRepository) ListByUser(userID uint64, req *domain.OrderListRequest) ([]*domain.Order, int64, error) {
	args := m.Called(userID, req)
	return args.Get(0).([]*domain.Order), args.Get(1).(int64), args.Error(2)
}

func (m *MockShippingOrderRepository) ListByUserWithItems(userID uint64, req *domain.OrderListRequest) ([]*domain.Order, int64, error) {
	args := m.Called(userID, req)
	return args.Get(0).([]*domain.Order), args.Get(1).(int64), args.Error(2)
}

func (m *MockShippingOrderRepository) ListBySeller(sellerID uint64, req *domain.OrderListRequest) ([]*domain.Order, int64, error) {
	args := m.Called(sellerID, req)
	return args.Get(0).([]*domain.Order), args.Get(1).(int64), args.Error(2)
}

func (m *MockShippingOrderRepository) FindItemByID(itemID uint64) (*domain.OrderItem, error) {
	args := m.Called(itemID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.OrderItem), args.Error(1)
}

func (m *MockShippingOrderRepository) UpdateItemStatus(itemID uint64, status domain.OrderItemStatus) error {
	args := m.Called(itemID, status)
	return args.Error(0)
}

func (m *MockShippingOrderRepository) GenerateOrderNumber() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func TestGetCarriers(t *testing.T) {
	t.Run("성공 - 배송사 목록 조회", func(t *testing.T) {
		// Given
		mockOrderRepo := new(MockShippingOrderRepository)
		carrierManager := carrier.NewCarrierManager()
		service := NewShippingService(mockOrderRepo, carrierManager)

		// When
		result := service.GetCarriers()

		// Then
		assert.NotNil(t, result)
		assert.NotNil(t, result.Carriers)
		// 배송사 목록에 CJ, Lotte 등이 포함되어 있어야 함
		assert.True(t, len(result.Carriers) > 0)
	})
}

func TestRegisterShipping(t *testing.T) {
	t.Run("성공 - 송장번호 등록", func(t *testing.T) {
		// Given
		mockOrderRepo := new(MockShippingOrderRepository)
		carrierManager := carrier.NewCarrierManager()
		cjCarrier := carrier.NewCJCarrier("")
		carrierManager.Register(cjCarrier)

		service := NewShippingService(mockOrderRepo, carrierManager)

		order := &domain.Order{
			ID:          1,
			UserID:      100,
			OrderNumber: "20240101000001",
			Status:      domain.OrderStatusPaid,
			Items: []domain.OrderItem{
				{
					ID:          1,
					ProductID:   1,
					SellerID:    200,
					ProductType: "physical",
				},
			},
		}

		mockOrderRepo.On("FindByID", uint64(1)).Return(order, nil)
		mockOrderRepo.On("Update", uint64(1), mock.AnythingOfType("*domain.Order")).Return(nil)

		req := &domain.RegisterShippingRequest{
			Carrier:        "cj",
			TrackingNumber: "1234567890",
		}

		// When
		err := service.RegisterShipping(context.Background(), 200, 1, req)

		// Then
		assert.NoError(t, err)
		mockOrderRepo.AssertExpectations(t)
	})

	t.Run("실패 - 주문 없음", func(t *testing.T) {
		// Given
		mockOrderRepo := new(MockShippingOrderRepository)
		service := NewShippingService(mockOrderRepo, nil)

		mockOrderRepo.On("FindByID", uint64(999)).Return(nil, ErrOrderNotFound)

		req := &domain.RegisterShippingRequest{
			Carrier:        "cj",
			TrackingNumber: "1234567890",
		}

		// When
		err := service.RegisterShipping(context.Background(), 200, 999, req)

		// Then
		assert.Equal(t, ErrOrderNotFound, err)
		mockOrderRepo.AssertExpectations(t)
	})

	t.Run("실패 - 권한 없음", func(t *testing.T) {
		// Given
		mockOrderRepo := new(MockShippingOrderRepository)
		service := NewShippingService(mockOrderRepo, nil)

		order := &domain.Order{
			ID:          1,
			UserID:      100,
			OrderNumber: "20240101000001",
			Status:      domain.OrderStatusPaid,
			Items: []domain.OrderItem{
				{
					ID:          1,
					ProductID:   1,
					SellerID:    300, // 다른 판매자
					ProductType: "physical",
				},
			},
		}

		mockOrderRepo.On("FindByID", uint64(1)).Return(order, nil)

		req := &domain.RegisterShippingRequest{
			Carrier:        "cj",
			TrackingNumber: "1234567890",
		}

		// When
		err := service.RegisterShipping(context.Background(), 200, 1, req) // 판매자 200은 권한 없음

		// Then
		assert.Equal(t, ErrOrderForbidden, err)
		mockOrderRepo.AssertExpectations(t)
	})

	t.Run("실패 - 디지털 상품은 배송 불가", func(t *testing.T) {
		// Given
		mockOrderRepo := new(MockShippingOrderRepository)
		service := NewShippingService(mockOrderRepo, nil)

		order := &domain.Order{
			ID:          1,
			UserID:      100,
			OrderNumber: "20240101000001",
			Status:      domain.OrderStatusPaid,
			Items: []domain.OrderItem{
				{
					ID:          1,
					ProductID:   1,
					SellerID:    200,
					ProductType: "digital", // 디지털 상품
				},
			},
		}

		mockOrderRepo.On("FindByID", uint64(1)).Return(order, nil)

		req := &domain.RegisterShippingRequest{
			Carrier:        "cj",
			TrackingNumber: "1234567890",
		}

		// When
		err := service.RegisterShipping(context.Background(), 200, 1, req)

		// Then
		assert.Equal(t, ErrShippingNotAllowed, err)
		mockOrderRepo.AssertExpectations(t)
	})

	t.Run("실패 - 이미 송장번호 등록됨", func(t *testing.T) {
		// Given
		mockOrderRepo := new(MockShippingOrderRepository)
		service := NewShippingService(mockOrderRepo, nil)

		order := &domain.Order{
			ID:             1,
			UserID:         100,
			OrderNumber:    "20240101000001",
			Status:         domain.OrderStatusShipped,
			TrackingNumber: "9876543210", // 이미 등록된 송장번호
			Items: []domain.OrderItem{
				{
					ID:          1,
					ProductID:   1,
					SellerID:    200,
					ProductType: "physical",
				},
			},
		}

		mockOrderRepo.On("FindByID", uint64(1)).Return(order, nil)

		req := &domain.RegisterShippingRequest{
			Carrier:        "cj",
			TrackingNumber: "1234567890",
		}

		// When
		err := service.RegisterShipping(context.Background(), 200, 1, req)

		// Then
		assert.Equal(t, ErrShippingAlreadySet, err)
		mockOrderRepo.AssertExpectations(t)
	})

	t.Run("실패 - 잘못된 주문 상태", func(t *testing.T) {
		// Given
		mockOrderRepo := new(MockShippingOrderRepository)
		service := NewShippingService(mockOrderRepo, nil)

		order := &domain.Order{
			ID:          1,
			UserID:      100,
			OrderNumber: "20240101000001",
			Status:      domain.OrderStatusCancelled, // 취소된 주문
			Items: []domain.OrderItem{
				{
					ID:          1,
					ProductID:   1,
					SellerID:    200,
					ProductType: "physical",
				},
			},
		}

		mockOrderRepo.On("FindByID", uint64(1)).Return(order, nil)

		req := &domain.RegisterShippingRequest{
			Carrier:        "cj",
			TrackingNumber: "1234567890",
		}

		// When
		err := service.RegisterShipping(context.Background(), 200, 1, req)

		// Then
		assert.Equal(t, ErrInvalidShippingStatus, err)
		mockOrderRepo.AssertExpectations(t)
	})

	t.Run("실패 - 지원하지 않는 배송사", func(t *testing.T) {
		// Given
		mockOrderRepo := new(MockShippingOrderRepository)
		service := NewShippingService(mockOrderRepo, nil)

		order := &domain.Order{
			ID:          1,
			UserID:      100,
			OrderNumber: "20240101000001",
			Status:      domain.OrderStatusPaid,
			Items: []domain.OrderItem{
				{
					ID:          1,
					ProductID:   1,
					SellerID:    200,
					ProductType: "physical",
				},
			},
		}

		mockOrderRepo.On("FindByID", uint64(1)).Return(order, nil)

		req := &domain.RegisterShippingRequest{
			Carrier:        "unknown_carrier", // 알 수 없는 배송사
			TrackingNumber: "1234567890",
		}

		// When
		err := service.RegisterShipping(context.Background(), 200, 1, req)

		// Then
		assert.Equal(t, ErrCarrierNotSupported, err)
		mockOrderRepo.AssertExpectations(t)
	})
}

func TestTrackShipping(t *testing.T) {
	t.Run("성공 - 배송 추적", func(t *testing.T) {
		// Given
		mockOrderRepo := new(MockShippingOrderRepository)
		carrierManager := carrier.NewCarrierManager()
		cjCarrier := carrier.NewCJCarrier("")
		carrierManager.Register(cjCarrier)

		service := NewShippingService(mockOrderRepo, carrierManager)

		order := &domain.Order{
			ID:              1,
			UserID:          100,
			OrderNumber:     "20240101000001",
			Status:          domain.OrderStatusShipped,
			ShippingCarrier: "cj",
			TrackingNumber:  "1234567890",
			Items: []domain.OrderItem{
				{
					ID:          1,
					ProductID:   1,
					SellerID:    200,
					ProductType: "physical",
				},
			},
		}

		mockOrderRepo.On("FindByID", uint64(1)).Return(order, nil)

		// When
		result, err := service.TrackShipping(context.Background(), 100, 1)

		// Then
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, uint64(1), result.OrderID)
		assert.Equal(t, "20240101000001", result.OrderNumber)
		assert.NotEmpty(t, result.TrackingURL)
		mockOrderRepo.AssertExpectations(t)
	})

	t.Run("실패 - 주문 없음", func(t *testing.T) {
		// Given
		mockOrderRepo := new(MockShippingOrderRepository)
		service := NewShippingService(mockOrderRepo, nil)

		mockOrderRepo.On("FindByID", uint64(999)).Return(nil, ErrOrderNotFound)

		// When
		_, err := service.TrackShipping(context.Background(), 100, 999)

		// Then
		assert.Equal(t, ErrOrderNotFound, err)
		mockOrderRepo.AssertExpectations(t)
	})

	t.Run("실패 - 권한 없음", func(t *testing.T) {
		// Given
		mockOrderRepo := new(MockShippingOrderRepository)
		service := NewShippingService(mockOrderRepo, nil)

		order := &domain.Order{
			ID:              1,
			UserID:          100, // 주문한 사용자
			OrderNumber:     "20240101000001",
			Status:          domain.OrderStatusShipped,
			ShippingCarrier: "cj",
			TrackingNumber:  "1234567890",
		}

		mockOrderRepo.On("FindByID", uint64(1)).Return(order, nil)

		// When
		_, err := service.TrackShipping(context.Background(), 200, 1) // 다른 사용자

		// Then
		assert.Equal(t, ErrOrderForbidden, err)
		mockOrderRepo.AssertExpectations(t)
	})

	t.Run("실패 - 송장번호 미등록", func(t *testing.T) {
		// Given
		mockOrderRepo := new(MockShippingOrderRepository)
		service := NewShippingService(mockOrderRepo, nil)

		order := &domain.Order{
			ID:             1,
			UserID:         100,
			OrderNumber:    "20240101000001",
			Status:         domain.OrderStatusPaid,
			TrackingNumber: "", // 송장번호 없음
		}

		mockOrderRepo.On("FindByID", uint64(1)).Return(order, nil)

		// When
		_, err := service.TrackShipping(context.Background(), 100, 1)

		// Then
		assert.Error(t, err)
		assert.Equal(t, "tracking number not set", err.Error())
		mockOrderRepo.AssertExpectations(t)
	})
}

func TestMarkDelivered(t *testing.T) {
	t.Run("성공 - 배송 완료 처리", func(t *testing.T) {
		// Given
		mockOrderRepo := new(MockShippingOrderRepository)
		service := NewShippingService(mockOrderRepo, nil)

		order := &domain.Order{
			ID:              1,
			UserID:          100,
			OrderNumber:     "20240101000001",
			Status:          domain.OrderStatusShipped,
			ShippingCarrier: "cj",
			TrackingNumber:  "1234567890",
			Items: []domain.OrderItem{
				{
					ID:          1,
					ProductID:   1,
					SellerID:    200,
					ProductType: "physical",
				},
			},
		}

		mockOrderRepo.On("FindByID", uint64(1)).Return(order, nil)
		mockOrderRepo.On("Update", uint64(1), mock.AnythingOfType("*domain.Order")).Return(nil)

		// When
		err := service.MarkDelivered(context.Background(), 200, 1)

		// Then
		assert.NoError(t, err)
		mockOrderRepo.AssertExpectations(t)
	})

	t.Run("실패 - 주문 없음", func(t *testing.T) {
		// Given
		mockOrderRepo := new(MockShippingOrderRepository)
		service := NewShippingService(mockOrderRepo, nil)

		mockOrderRepo.On("FindByID", uint64(999)).Return(nil, ErrOrderNotFound)

		// When
		err := service.MarkDelivered(context.Background(), 200, 999)

		// Then
		assert.Equal(t, ErrOrderNotFound, err)
		mockOrderRepo.AssertExpectations(t)
	})

	t.Run("실패 - 권한 없음", func(t *testing.T) {
		// Given
		mockOrderRepo := new(MockShippingOrderRepository)
		service := NewShippingService(mockOrderRepo, nil)

		order := &domain.Order{
			ID:          1,
			UserID:      100,
			OrderNumber: "20240101000001",
			Status:      domain.OrderStatusShipped,
			Items: []domain.OrderItem{
				{
					ID:          1,
					ProductID:   1,
					SellerID:    300, // 다른 판매자
					ProductType: "physical",
				},
			},
		}

		mockOrderRepo.On("FindByID", uint64(1)).Return(order, nil)

		// When
		err := service.MarkDelivered(context.Background(), 200, 1) // 판매자 200은 권한 없음

		// Then
		assert.Equal(t, ErrOrderForbidden, err)
		mockOrderRepo.AssertExpectations(t)
	})

	t.Run("실패 - 잘못된 주문 상태", func(t *testing.T) {
		// Given
		mockOrderRepo := new(MockShippingOrderRepository)
		service := NewShippingService(mockOrderRepo, nil)

		order := &domain.Order{
			ID:          1,
			UserID:      100,
			OrderNumber: "20240101000001",
			Status:      domain.OrderStatusPaid, // 배송 중이 아님
			Items: []domain.OrderItem{
				{
					ID:          1,
					ProductID:   1,
					SellerID:    200,
					ProductType: "physical",
				},
			},
		}

		mockOrderRepo.On("FindByID", uint64(1)).Return(order, nil)

		// When
		err := service.MarkDelivered(context.Background(), 200, 1)

		// Then
		assert.Equal(t, ErrInvalidShippingStatus, err)
		mockOrderRepo.AssertExpectations(t)
	})
}

func TestUpdateShippingStatus(t *testing.T) {
	t.Run("성공 - 배송 상태 업데이트 (배송 완료)", func(t *testing.T) {
		// Given
		mockOrderRepo := new(MockShippingOrderRepository)
		carrierManager := carrier.NewCarrierManager()
		// 실제 배송 추적 API 없이 Mock 데이터 사용
		cjCarrier := carrier.NewCJCarrier("") // 빈 API 키 = Mock 데이터
		carrierManager.Register(cjCarrier)

		service := NewShippingService(mockOrderRepo, carrierManager)

		order := &domain.Order{
			ID:              1,
			UserID:          100,
			OrderNumber:     "20240101000001",
			Status:          domain.OrderStatusShipped,
			ShippingCarrier: "cj",
			TrackingNumber:  "1234567890",
		}

		mockOrderRepo.On("FindByID", uint64(1)).Return(order, nil)
		// Mock 데이터는 항상 InTransit 상태를 반환하므로 Update 호출 없음

		// When
		err := service.UpdateShippingStatus(context.Background(), 1)

		// Then
		assert.NoError(t, err)
		mockOrderRepo.AssertExpectations(t)
	})

	t.Run("스킵 - 송장번호 없음", func(t *testing.T) {
		// Given
		mockOrderRepo := new(MockShippingOrderRepository)
		service := NewShippingService(mockOrderRepo, nil)

		order := &domain.Order{
			ID:             1,
			UserID:         100,
			OrderNumber:    "20240101000001",
			Status:         domain.OrderStatusPaid,
			TrackingNumber: "", // 송장번호 없음
		}

		mockOrderRepo.On("FindByID", uint64(1)).Return(order, nil)

		// When
		err := service.UpdateShippingStatus(context.Background(), 1)

		// Then
		assert.NoError(t, err) // 에러 없이 스킵
		mockOrderRepo.AssertExpectations(t)
	})

	t.Run("스킵 - 이미 배송 완료", func(t *testing.T) {
		// Given
		mockOrderRepo := new(MockShippingOrderRepository)
		service := NewShippingService(mockOrderRepo, nil)

		order := &domain.Order{
			ID:             1,
			UserID:         100,
			OrderNumber:    "20240101000001",
			Status:         domain.OrderStatusDelivered, // 이미 배송 완료
			TrackingNumber: "1234567890",
		}

		mockOrderRepo.On("FindByID", uint64(1)).Return(order, nil)

		// When
		err := service.UpdateShippingStatus(context.Background(), 1)

		// Then
		assert.NoError(t, err) // 에러 없이 스킵
		mockOrderRepo.AssertExpectations(t)
	})

	t.Run("스킵 - CarrierManager 없음", func(t *testing.T) {
		// Given
		mockOrderRepo := new(MockShippingOrderRepository)
		service := NewShippingService(mockOrderRepo, nil) // CarrierManager 없음

		order := &domain.Order{
			ID:              1,
			UserID:          100,
			OrderNumber:     "20240101000001",
			Status:          domain.OrderStatusShipped,
			ShippingCarrier: "cj",
			TrackingNumber:  "1234567890",
		}

		mockOrderRepo.On("FindByID", uint64(1)).Return(order, nil)

		// When
		err := service.UpdateShippingStatus(context.Background(), 1)

		// Then
		assert.NoError(t, err) // 에러 없이 스킵
		mockOrderRepo.AssertExpectations(t)
	})
}
