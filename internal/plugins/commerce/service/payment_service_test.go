package service

import (
	"context"
	"testing"
	"time"

	"github.com/damoang/angple-backend/internal/plugins/commerce/domain"
	"github.com/damoang/angple-backend/internal/plugins/commerce/gateway"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"gorm.io/gorm"
)

// MockPaymentRepository 결제 저장소 목
type MockPaymentRepository struct {
	mock.Mock
}

func (m *MockPaymentRepository) Create(payment *domain.Payment) error {
	args := m.Called(payment)
	return args.Error(0)
}

func (m *MockPaymentRepository) Update(id uint64, payment *domain.Payment) error {
	args := m.Called(id, payment)
	return args.Error(0)
}

func (m *MockPaymentRepository) UpdateStatus(id uint64, status domain.PaymentStatus) error {
	args := m.Called(id, status)
	return args.Error(0)
}

func (m *MockPaymentRepository) UpdatePaid(id uint64, paidAt time.Time, pgFee float64) error {
	args := m.Called(id, paidAt, pgFee)
	return args.Error(0)
}

func (m *MockPaymentRepository) UpdateCancelled(id uint64, cancelledAmount float64, cancelReason string) error {
	args := m.Called(id, cancelledAmount, cancelReason)
	return args.Error(0)
}

func (m *MockPaymentRepository) FindByID(id uint64) (*domain.Payment, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Payment), args.Error(1)
}

func (m *MockPaymentRepository) FindByIDWithOrder(id uint64) (*domain.Payment, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Payment), args.Error(1)
}

func (m *MockPaymentRepository) FindByOrderID(orderID uint64) (*domain.Payment, error) {
	args := m.Called(orderID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Payment), args.Error(1)
}

func (m *MockPaymentRepository) FindByPGTID(provider domain.PGProvider, pgTID string) (*domain.Payment, error) {
	args := m.Called(provider, pgTID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Payment), args.Error(1)
}

func (m *MockPaymentRepository) FindByPGOrderID(provider domain.PGProvider, pgOrderID string) (*domain.Payment, error) {
	args := m.Called(provider, pgOrderID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Payment), args.Error(1)
}

func (m *MockPaymentRepository) ListByOrderID(orderID uint64) ([]*domain.Payment, error) {
	args := m.Called(orderID)
	return args.Get(0).([]*domain.Payment), args.Error(1)
}

// MockOrderRepositoryForPayment 주문 저장소 목 (결제용)
type MockOrderRepositoryForPayment struct {
	mock.Mock
}

func (m *MockOrderRepositoryForPayment) Create(order *domain.Order) error {
	args := m.Called(order)
	return args.Error(0)
}

func (m *MockOrderRepositoryForPayment) CreateWithItems(order *domain.Order, items []domain.OrderItem) error {
	args := m.Called(order, items)
	return args.Error(0)
}

func (m *MockOrderRepositoryForPayment) Update(id uint64, order *domain.Order) error {
	args := m.Called(id, order)
	return args.Error(0)
}

func (m *MockOrderRepositoryForPayment) UpdateStatus(id uint64, status domain.OrderStatus) error {
	args := m.Called(id, status)
	return args.Error(0)
}

func (m *MockOrderRepositoryForPayment) FindByID(id uint64) (*domain.Order, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Order), args.Error(1)
}

func (m *MockOrderRepositoryForPayment) FindByIDWithItems(id uint64) (*domain.Order, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Order), args.Error(1)
}

func (m *MockOrderRepositoryForPayment) FindByOrderNumber(orderNumber string) (*domain.Order, error) {
	args := m.Called(orderNumber)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Order), args.Error(1)
}

func (m *MockOrderRepositoryForPayment) FindByOrderNumberWithItems(orderNumber string) (*domain.Order, error) {
	args := m.Called(orderNumber)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Order), args.Error(1)
}

func (m *MockOrderRepositoryForPayment) ListByUser(userID uint64, req *domain.OrderListRequest) ([]*domain.Order, int64, error) {
	args := m.Called(userID, req)
	return args.Get(0).([]*domain.Order), args.Get(1).(int64), args.Error(2)
}

func (m *MockOrderRepositoryForPayment) ListByUserWithItems(userID uint64, req *domain.OrderListRequest) ([]*domain.Order, int64, error) {
	args := m.Called(userID, req)
	return args.Get(0).([]*domain.Order), args.Get(1).(int64), args.Error(2)
}

func (m *MockOrderRepositoryForPayment) ListBySeller(sellerID uint64, req *domain.OrderListRequest) ([]*domain.Order, int64, error) {
	args := m.Called(sellerID, req)
	return args.Get(0).([]*domain.Order), args.Get(1).(int64), args.Error(2)
}

func (m *MockOrderRepositoryForPayment) FindItemByID(itemID uint64) (*domain.OrderItem, error) {
	args := m.Called(itemID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.OrderItem), args.Error(1)
}

func (m *MockOrderRepositoryForPayment) UpdateItemStatus(itemID uint64, status domain.OrderItemStatus) error {
	args := m.Called(itemID, status)
	return args.Error(0)
}

func (m *MockOrderRepositoryForPayment) GenerateOrderNumber() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

// MockGateway 결제 게이트웨이 목
type MockGateway struct {
	mock.Mock
}

func (m *MockGateway) Provider() domain.PGProvider {
	args := m.Called()
	return args.Get(0).(domain.PGProvider)
}

func (m *MockGateway) Prepare(ctx context.Context, req *gateway.PrepareRequest) (*gateway.PrepareResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*gateway.PrepareResponse), args.Error(1)
}

func (m *MockGateway) Complete(ctx context.Context, req *gateway.CompleteRequest) (*gateway.CompleteResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*gateway.CompleteResponse), args.Error(1)
}

func (m *MockGateway) Cancel(ctx context.Context, req *gateway.CancelRequest) (*gateway.CancelResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*gateway.CancelResponse), args.Error(1)
}

func (m *MockGateway) HandleWebhook(ctx context.Context, payload []byte) (*gateway.WebhookResult, error) {
	args := m.Called(ctx, payload)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*gateway.WebhookResult), args.Error(1)
}

func (m *MockGateway) Verify(ctx context.Context, pgTID string, amount float64) error {
	args := m.Called(ctx, pgTID, amount)
	return args.Error(0)
}

func TestPreparePayment(t *testing.T) {
	t.Run("성공 - 결제 준비", func(t *testing.T) {
		paymentRepo := new(MockPaymentRepository)
		orderRepo := new(MockOrderRepositoryForPayment)
		productRepo := new(MockProductRepository)
		mockGateway := new(MockGateway)

		gatewayMgr := gateway.NewGatewayManager()
		mockGateway.On("Provider").Return(domain.PGProviderTossPayments)
		gatewayMgr.Register(mockGateway)

		svc := NewPaymentService(paymentRepo, orderRepo, productRepo, gatewayMgr)

		order := &domain.Order{
			ID:     1,
			UserID: 1,
			Status: domain.OrderStatusPending,
			Total:  10000,
			Items: []domain.OrderItem{
				{ID: 1, ProductName: "테스트 상품", Quantity: 1, Price: 10000},
			},
		}

		orderRepo.On("FindByIDWithItems", uint64(1)).Return(order, nil)
		paymentRepo.On("FindByOrderID", uint64(1)).Return(nil, gorm.ErrRecordNotFound)
		paymentRepo.On("Create", mock.AnythingOfType("*domain.Payment")).Return(nil)
		mockGateway.On("Prepare", mock.Anything, mock.AnythingOfType("*gateway.PrepareRequest")).Return(&gateway.PrepareResponse{
			PGOrderID:   "PG-ORD-001",
			RedirectURL: "https://pay.example.com",
		}, nil)

		ctx := context.Background()
		req := &domain.PreparePaymentRequest{
			OrderID:       1,
			PGProvider:    "tosspayments",
			PaymentMethod: "card",
			ReturnURL:     "https://example.com/return",
		}

		result, err := svc.PreparePayment(ctx, 1, req)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "PG-ORD-001", result.PGOrderID)
	})

	t.Run("실패 - 주문 없음", func(t *testing.T) {
		paymentRepo := new(MockPaymentRepository)
		orderRepo := new(MockOrderRepositoryForPayment)
		productRepo := new(MockProductRepository)
		gatewayMgr := gateway.NewGatewayManager()

		svc := NewPaymentService(paymentRepo, orderRepo, productRepo, gatewayMgr)

		orderRepo.On("FindByIDWithItems", uint64(999)).Return(nil, gorm.ErrRecordNotFound)

		ctx := context.Background()
		req := &domain.PreparePaymentRequest{
			OrderID:       999,
			PGProvider:    "tosspayments",
			PaymentMethod: "card",
			ReturnURL:     "https://example.com/return",
		}

		result, err := svc.PreparePayment(ctx, 1, req)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, ErrOrderNotFound, err)
	})

	t.Run("실패 - 다른 사용자의 주문", func(t *testing.T) {
		paymentRepo := new(MockPaymentRepository)
		orderRepo := new(MockOrderRepositoryForPayment)
		productRepo := new(MockProductRepository)
		gatewayMgr := gateway.NewGatewayManager()

		svc := NewPaymentService(paymentRepo, orderRepo, productRepo, gatewayMgr)

		order := &domain.Order{
			ID:     1,
			UserID: 2, // 다른 사용자
			Status: domain.OrderStatusPending,
		}

		orderRepo.On("FindByIDWithItems", uint64(1)).Return(order, nil)

		ctx := context.Background()
		req := &domain.PreparePaymentRequest{
			OrderID:       1,
			PGProvider:    "tosspayments",
			PaymentMethod: "card",
			ReturnURL:     "https://example.com/return",
		}

		result, err := svc.PreparePayment(ctx, 1, req) // 사용자 ID: 1

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, ErrOrderForbidden, err)
	})
}

func TestGetPayment(t *testing.T) {
	t.Run("성공 - 결제 조회", func(t *testing.T) {
		paymentRepo := new(MockPaymentRepository)
		orderRepo := new(MockOrderRepositoryForPayment)
		productRepo := new(MockProductRepository)
		gatewayMgr := gateway.NewGatewayManager()

		svc := NewPaymentService(paymentRepo, orderRepo, productRepo, gatewayMgr)

		order := &domain.Order{
			ID:          1,
			UserID:      1,
			OrderNumber: "ORD-001",
		}
		payment := &domain.Payment{
			ID:         1,
			OrderID:    1,
			Amount:     10000,
			Status:     domain.PaymentStatusPaid,
			PGProvider: domain.PGProviderTossPayments,
			CreatedAt:  time.Now(),
			Order:      order,
		}

		paymentRepo.On("FindByIDWithOrder", uint64(1)).Return(payment, nil)

		ctx := context.Background()
		result, err := svc.GetPayment(ctx, 1, 1)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, uint64(1), result.ID)
		assert.Equal(t, float64(10000), result.Amount)
	})

	t.Run("실패 - 결제 없음", func(t *testing.T) {
		paymentRepo := new(MockPaymentRepository)
		orderRepo := new(MockOrderRepositoryForPayment)
		productRepo := new(MockProductRepository)
		gatewayMgr := gateway.NewGatewayManager()

		svc := NewPaymentService(paymentRepo, orderRepo, productRepo, gatewayMgr)

		paymentRepo.On("FindByIDWithOrder", uint64(999)).Return(nil, gorm.ErrRecordNotFound)

		ctx := context.Background()
		result, err := svc.GetPayment(ctx, 1, 999)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, ErrPaymentNotFound, err)
	})

	t.Run("실패 - 다른 사용자의 결제", func(t *testing.T) {
		paymentRepo := new(MockPaymentRepository)
		orderRepo := new(MockOrderRepositoryForPayment)
		productRepo := new(MockProductRepository)
		gatewayMgr := gateway.NewGatewayManager()

		svc := NewPaymentService(paymentRepo, orderRepo, productRepo, gatewayMgr)

		order := &domain.Order{
			ID:     1,
			UserID: 2, // 다른 사용자
		}
		payment := &domain.Payment{
			ID:      1,
			OrderID: 1,
			Order:   order,
		}

		paymentRepo.On("FindByIDWithOrder", uint64(1)).Return(payment, nil)

		ctx := context.Background()
		result, err := svc.GetPayment(ctx, 1, 1) // 사용자 ID: 1

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, ErrPaymentForbidden, err)
	})
}
