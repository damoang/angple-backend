package service

import (
	"testing"

	"github.com/damoang/angple-backend/internal/plugins/commerce/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"gorm.io/gorm"
)

// MockOrderRepository 주문 저장소 모의 객체
type MockOrderRepository struct {
	mock.Mock
}

func (m *MockOrderRepository) Create(order *domain.Order) error {
	args := m.Called(order)
	return args.Error(0)
}

func (m *MockOrderRepository) CreateWithItems(order *domain.Order, items []domain.OrderItem) error {
	args := m.Called(order, items)
	if args.Error(0) == nil {
		order.ID = 1 // 생성된 주문 ID 시뮬레이션
	}
	return args.Error(0)
}

func (m *MockOrderRepository) Update(id uint64, order *domain.Order) error {
	args := m.Called(id, order)
	return args.Error(0)
}

func (m *MockOrderRepository) UpdateStatus(id uint64, status domain.OrderStatus) error {
	args := m.Called(id, status)
	return args.Error(0)
}

func (m *MockOrderRepository) FindByID(id uint64) (*domain.Order, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Order), args.Error(1)
}

func (m *MockOrderRepository) FindByIDWithItems(id uint64) (*domain.Order, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Order), args.Error(1)
}

func (m *MockOrderRepository) FindByOrderNumber(orderNumber string) (*domain.Order, error) {
	args := m.Called(orderNumber)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Order), args.Error(1)
}

func (m *MockOrderRepository) FindByOrderNumberWithItems(orderNumber string) (*domain.Order, error) {
	args := m.Called(orderNumber)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Order), args.Error(1)
}

func (m *MockOrderRepository) ListByUser(userID uint64, req *domain.OrderListRequest) ([]*domain.Order, int64, error) {
	args := m.Called(userID, req)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}
	return args.Get(0).([]*domain.Order), args.Get(1).(int64), args.Error(2)
}

func (m *MockOrderRepository) ListByUserWithItems(userID uint64, req *domain.OrderListRequest) ([]*domain.Order, int64, error) {
	args := m.Called(userID, req)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}
	return args.Get(0).([]*domain.Order), args.Get(1).(int64), args.Error(2)
}

func (m *MockOrderRepository) ListBySeller(sellerID uint64, req *domain.OrderListRequest) ([]*domain.Order, int64, error) {
	args := m.Called(sellerID, req)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}
	return args.Get(0).([]*domain.Order), args.Get(1).(int64), args.Error(2)
}

func (m *MockOrderRepository) FindItemByID(itemID uint64) (*domain.OrderItem, error) {
	args := m.Called(itemID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.OrderItem), args.Error(1)
}

func (m *MockOrderRepository) UpdateItemStatus(itemID uint64, status domain.OrderItemStatus) error {
	args := m.Called(itemID, status)
	return args.Error(0)
}

func (m *MockOrderRepository) GenerateOrderNumber() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func TestCreateOrder(t *testing.T) {
	t.Run("성공 - 디지털 상품 주문 생성", func(t *testing.T) {
		orderRepo := new(MockOrderRepository)
		cartRepo := new(MockCartRepository)
		productRepo := new(MockProductRepository)
		svc := NewOrderService(orderRepo, cartRepo, productRepo)

		product := &domain.Product{
			ID:          1,
			SellerID:    100,
			Name:        "테스트 디지털 상품",
			Price:       10000,
			ProductType: domain.ProductTypeDigital,
			Status:      domain.ProductStatusPublished,
			Currency:    "KRW",
		}

		carts := []*domain.Cart{
			{
				ID:        1,
				UserID:    1,
				ProductID: 1,
				Quantity:  2,
				Product:   product,
			},
		}

		createdOrder := &domain.Order{
			ID:          1,
			OrderNumber: "20240101120000123456",
			UserID:      1,
			Subtotal:    20000,
			Total:       20000,
			Currency:    "KRW",
			Status:      domain.OrderStatusPending,
			Items: []domain.OrderItem{
				{
					ID:          1,
					ProductID:   1,
					ProductName: "테스트 디지털 상품",
					ProductType: domain.ProductTypeDigital,
					Price:       10000,
					Quantity:    2,
					Subtotal:    20000,
				},
			},
		}

		cartRepo.On("ListByUserWithProducts", uint64(1)).Return(carts, nil)
		orderRepo.On("GenerateOrderNumber").Return("20240101120000123456", nil)
		orderRepo.On("CreateWithItems", mock.AnythingOfType("*domain.Order"), mock.AnythingOfType("[]domain.OrderItem")).Return(nil)
		orderRepo.On("FindByIDWithItems", uint64(1)).Return(createdOrder, nil)
		cartRepo.On("DeleteByUserID", uint64(1)).Return(nil)

		req := &domain.CreateOrderRequest{}
		order, err := svc.CreateOrder(1, req, "127.0.0.1", "TestAgent")

		assert.NoError(t, err)
		assert.NotNil(t, order)
		assert.Equal(t, "20240101120000123456", order.OrderNumber)
		assert.Equal(t, float64(20000), order.Total)
		assert.Len(t, order.Items, 1)
		cartRepo.AssertExpectations(t)
		orderRepo.AssertExpectations(t)
	})

	t.Run("실패 - 빈 장바구니", func(t *testing.T) {
		orderRepo := new(MockOrderRepository)
		cartRepo := new(MockCartRepository)
		productRepo := new(MockProductRepository)
		svc := NewOrderService(orderRepo, cartRepo, productRepo)

		cartRepo.On("ListByUserWithProducts", uint64(1)).Return([]*domain.Cart{}, nil)

		req := &domain.CreateOrderRequest{}
		order, err := svc.CreateOrder(1, req, "127.0.0.1", "TestAgent")

		assert.Error(t, err)
		assert.Nil(t, order)
		assert.Equal(t, ErrEmptyCart, err)
		cartRepo.AssertExpectations(t)
	})

	t.Run("실패 - 실물 상품 배송정보 누락", func(t *testing.T) {
		orderRepo := new(MockOrderRepository)
		cartRepo := new(MockCartRepository)
		productRepo := new(MockProductRepository)
		svc := NewOrderService(orderRepo, cartRepo, productRepo)

		product := &domain.Product{
			ID:          1,
			SellerID:    100,
			Name:        "테스트 실물 상품",
			Price:       10000,
			ProductType: domain.ProductTypePhysical,
			Status:      domain.ProductStatusPublished,
		}

		carts := []*domain.Cart{
			{
				ID:        1,
				UserID:    1,
				ProductID: 1,
				Quantity:  1,
				Product:   product,
			},
		}

		cartRepo.On("ListByUserWithProducts", uint64(1)).Return(carts, nil)

		req := &domain.CreateOrderRequest{} // 배송 정보 없음
		order, err := svc.CreateOrder(1, req, "127.0.0.1", "TestAgent")

		assert.Error(t, err)
		assert.Nil(t, order)
		assert.Equal(t, ErrShippingInfoRequired, err)
		cartRepo.AssertExpectations(t)
	})
}

func TestGetOrder(t *testing.T) {
	t.Run("성공 - 주문 조회", func(t *testing.T) {
		orderRepo := new(MockOrderRepository)
		cartRepo := new(MockCartRepository)
		productRepo := new(MockProductRepository)
		svc := NewOrderService(orderRepo, cartRepo, productRepo)

		order := &domain.Order{
			ID:          1,
			OrderNumber: "20240101120000123456",
			UserID:      1,
			Subtotal:    20000,
			Total:       20000,
			Currency:    "KRW",
			Status:      domain.OrderStatusPending,
			Items:       []domain.OrderItem{},
		}

		orderRepo.On("FindByIDWithItems", uint64(1)).Return(order, nil)

		result, err := svc.GetOrder(1, 1)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "20240101120000123456", result.OrderNumber)
		orderRepo.AssertExpectations(t)
	})

	t.Run("실패 - 주문 없음", func(t *testing.T) {
		orderRepo := new(MockOrderRepository)
		cartRepo := new(MockCartRepository)
		productRepo := new(MockProductRepository)
		svc := NewOrderService(orderRepo, cartRepo, productRepo)

		orderRepo.On("FindByIDWithItems", uint64(1)).Return(nil, gorm.ErrRecordNotFound)

		result, err := svc.GetOrder(1, 1)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, ErrOrderNotFound, err)
		orderRepo.AssertExpectations(t)
	})

	t.Run("실패 - 다른 사용자의 주문", func(t *testing.T) {
		orderRepo := new(MockOrderRepository)
		cartRepo := new(MockCartRepository)
		productRepo := new(MockProductRepository)
		svc := NewOrderService(orderRepo, cartRepo, productRepo)

		order := &domain.Order{
			ID:          1,
			OrderNumber: "20240101120000123456",
			UserID:      2, // 다른 사용자
			Subtotal:    20000,
			Total:       20000,
		}

		orderRepo.On("FindByIDWithItems", uint64(1)).Return(order, nil)

		result, err := svc.GetOrder(1, 1)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, ErrOrderForbidden, err)
		orderRepo.AssertExpectations(t)
	})
}

func TestCancelOrder(t *testing.T) {
	t.Run("성공 - 주문 취소", func(t *testing.T) {
		orderRepo := new(MockOrderRepository)
		cartRepo := new(MockCartRepository)
		productRepo := new(MockProductRepository)
		svc := NewOrderService(orderRepo, cartRepo, productRepo)

		order := &domain.Order{
			ID:          1,
			OrderNumber: "20240101120000123456",
			UserID:      1,
			Status:      domain.OrderStatusPending,
			Items: []domain.OrderItem{
				{
					ID:          1,
					ProductID:   1,
					ProductType: domain.ProductTypeDigital, // 디지털 상품 - 재고 복구 불필요
				},
			},
		}

		orderRepo.On("FindByIDWithItems", uint64(1)).Return(order, nil)
		orderRepo.On("UpdateStatus", uint64(1), domain.OrderStatusCancelled).Return(nil)

		err := svc.CancelOrder(1, 1, &domain.CancelOrderRequest{Reason: "테스트 취소"})

		assert.NoError(t, err)
		orderRepo.AssertExpectations(t)
	})

	t.Run("실패 - 취소 불가 상태", func(t *testing.T) {
		orderRepo := new(MockOrderRepository)
		cartRepo := new(MockCartRepository)
		productRepo := new(MockProductRepository)
		svc := NewOrderService(orderRepo, cartRepo, productRepo)

		order := &domain.Order{
			ID:          1,
			OrderNumber: "20240101120000123456",
			UserID:      1,
			Status:      domain.OrderStatusPaid, // 결제 완료 상태
			Items:       []domain.OrderItem{},
		}

		orderRepo.On("FindByIDWithItems", uint64(1)).Return(order, nil)

		err := svc.CancelOrder(1, 1, &domain.CancelOrderRequest{})

		assert.Error(t, err)
		assert.Equal(t, ErrOrderCannotBeCancelled, err)
		orderRepo.AssertExpectations(t)
	})

	t.Run("성공 - 실물 상품 주문 취소 시 재고 복구", func(t *testing.T) {
		orderRepo := new(MockOrderRepository)
		cartRepo := new(MockCartRepository)
		productRepo := new(MockProductRepository)
		svc := NewOrderService(orderRepo, cartRepo, productRepo)

		stock := 5
		product := &domain.Product{
			ID:            1,
			Name:          "실물 상품",
			ProductType:   domain.ProductTypePhysical,
			StockQuantity: &stock,
			StockStatus:   domain.StockStatusOutOfStock,
		}

		order := &domain.Order{
			ID:          1,
			OrderNumber: "20240101120000123456",
			UserID:      1,
			Status:      domain.OrderStatusPending,
			Items: []domain.OrderItem{
				{
					ID:          1,
					ProductID:   1,
					ProductType: domain.ProductTypePhysical,
					Quantity:    3,
				},
			},
		}

		orderRepo.On("FindByIDWithItems", uint64(1)).Return(order, nil)
		productRepo.On("FindByID", uint64(1)).Return(product, nil)
		productRepo.On("Update", uint64(1), mock.AnythingOfType("*domain.Product")).Return(nil)
		orderRepo.On("UpdateStatus", uint64(1), domain.OrderStatusCancelled).Return(nil)

		err := svc.CancelOrder(1, 1, &domain.CancelOrderRequest{})

		assert.NoError(t, err)
		// 재고가 복구되었는지 확인
		assert.Equal(t, 8, *product.StockQuantity) // 5 + 3
		assert.Equal(t, domain.StockStatusInStock, product.StockStatus)
		orderRepo.AssertExpectations(t)
		productRepo.AssertExpectations(t)
	})
}

func TestListOrders(t *testing.T) {
	t.Run("성공 - 주문 목록 조회", func(t *testing.T) {
		orderRepo := new(MockOrderRepository)
		cartRepo := new(MockCartRepository)
		productRepo := new(MockProductRepository)
		svc := NewOrderService(orderRepo, cartRepo, productRepo)

		orders := []*domain.Order{
			{
				ID:          1,
				OrderNumber: "20240101120000123456",
				UserID:      1,
				Total:       20000,
				Currency:    "KRW",
				Status:      domain.OrderStatusPending,
				Items:       []domain.OrderItem{},
			},
		}

		orderRepo.On("ListByUserWithItems", uint64(1), mock.AnythingOfType("*domain.OrderListRequest")).Return(orders, int64(1), nil)

		req := &domain.OrderListRequest{Page: 1, Limit: 20}
		result, meta, err := svc.ListOrders(1, req)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Len(t, result, 1)
		assert.Equal(t, int64(1), meta.Total)
		orderRepo.AssertExpectations(t)
	})
}
