package service

import (
	"testing"
	"time"

	"github.com/damoang/angple-backend/internal/plugins/commerce/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"gorm.io/gorm"
)

// MockSettlementRepository 정산 저장소 목
type MockSettlementRepository struct {
	mock.Mock
}

func (m *MockSettlementRepository) Create(settlement *domain.Settlement) error {
	args := m.Called(settlement)
	return args.Error(0)
}

func (m *MockSettlementRepository) Update(id uint64, settlement *domain.Settlement) error {
	args := m.Called(id, settlement)
	return args.Error(0)
}

func (m *MockSettlementRepository) FindByID(id uint64) (*domain.Settlement, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Settlement), args.Error(1)
}

func (m *MockSettlementRepository) FindBySellerAndPeriod(sellerID uint64, periodStart, periodEnd time.Time) (*domain.Settlement, error) {
	args := m.Called(sellerID, periodStart, periodEnd)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Settlement), args.Error(1)
}

func (m *MockSettlementRepository) ListBySeller(sellerID uint64, req *domain.SettlementListRequest) ([]*domain.Settlement, int64, error) {
	args := m.Called(sellerID, req)
	return args.Get(0).([]*domain.Settlement), args.Get(1).(int64), args.Error(2)
}

func (m *MockSettlementRepository) ListAll(req *domain.SettlementListRequest) ([]*domain.Settlement, int64, error) {
	args := m.Called(req)
	return args.Get(0).([]*domain.Settlement), args.Get(1).(int64), args.Error(2)
}

func (m *MockSettlementRepository) ListPending() ([]*domain.Settlement, error) {
	args := m.Called()
	return args.Get(0).([]*domain.Settlement), args.Error(1)
}

func (m *MockSettlementRepository) GetSummaryBySeller(sellerID uint64) (*domain.SettlementSummary, error) {
	args := m.Called(sellerID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.SettlementSummary), args.Error(1)
}

func (m *MockSettlementRepository) GetPendingSettlementOrders(sellerID uint64, periodStart, periodEnd time.Time) ([]*domain.OrderItem, error) {
	args := m.Called(sellerID, periodStart, periodEnd)
	return args.Get(0).([]*domain.OrderItem), args.Error(1)
}

// MockOrderRepositoryForSettlement 주문 저장소 목 (정산용)
type MockOrderRepositoryForSettlement struct {
	mock.Mock
}

func (m *MockOrderRepositoryForSettlement) Create(order *domain.Order) error {
	args := m.Called(order)
	return args.Error(0)
}

func (m *MockOrderRepositoryForSettlement) CreateWithItems(order *domain.Order, items []domain.OrderItem) error {
	args := m.Called(order, items)
	return args.Error(0)
}

func (m *MockOrderRepositoryForSettlement) Update(id uint64, order *domain.Order) error {
	args := m.Called(id, order)
	return args.Error(0)
}

func (m *MockOrderRepositoryForSettlement) UpdateStatus(id uint64, status domain.OrderStatus) error {
	args := m.Called(id, status)
	return args.Error(0)
}

func (m *MockOrderRepositoryForSettlement) FindByID(id uint64) (*domain.Order, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Order), args.Error(1)
}

func (m *MockOrderRepositoryForSettlement) FindByIDWithItems(id uint64) (*domain.Order, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Order), args.Error(1)
}

func (m *MockOrderRepositoryForSettlement) FindByOrderNumber(orderNumber string) (*domain.Order, error) {
	args := m.Called(orderNumber)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Order), args.Error(1)
}

func (m *MockOrderRepositoryForSettlement) FindByOrderNumberWithItems(orderNumber string) (*domain.Order, error) {
	args := m.Called(orderNumber)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Order), args.Error(1)
}

func (m *MockOrderRepositoryForSettlement) ListByUser(userID uint64, req *domain.OrderListRequest) ([]*domain.Order, int64, error) {
	args := m.Called(userID, req)
	return args.Get(0).([]*domain.Order), args.Get(1).(int64), args.Error(2)
}

func (m *MockOrderRepositoryForSettlement) ListByUserWithItems(userID uint64, req *domain.OrderListRequest) ([]*domain.Order, int64, error) {
	args := m.Called(userID, req)
	return args.Get(0).([]*domain.Order), args.Get(1).(int64), args.Error(2)
}

func (m *MockOrderRepositoryForSettlement) ListBySeller(sellerID uint64, req *domain.OrderListRequest) ([]*domain.Order, int64, error) {
	args := m.Called(sellerID, req)
	return args.Get(0).([]*domain.Order), args.Get(1).(int64), args.Error(2)
}

func (m *MockOrderRepositoryForSettlement) FindItemByID(itemID uint64) (*domain.OrderItem, error) {
	args := m.Called(itemID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.OrderItem), args.Error(1)
}

func (m *MockOrderRepositoryForSettlement) UpdateItemStatus(itemID uint64, status domain.OrderItemStatus) error {
	args := m.Called(itemID, status)
	return args.Error(0)
}

func (m *MockOrderRepositoryForSettlement) GenerateOrderNumber() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func TestCreateSettlement(t *testing.T) {
	t.Run("성공 - 정산 생성", func(t *testing.T) {
		settlementRepo := new(MockSettlementRepository)
		orderRepo := new(MockOrderRepositoryForSettlement)

		svc := NewSettlementService(settlementRepo, orderRepo)

		periodStart := time.Now().AddDate(0, -1, 0)
		periodEnd := time.Now()

		orders := []*domain.OrderItem{
			{
				ID:       1,
				SellerID: 1,
				Price:    10000,
				Quantity: 1,
				Subtotal: 10000,
			},
			{
				ID:       2,
				SellerID: 1,
				Price:    20000,
				Quantity: 2,
				Subtotal: 40000,
			},
		}

		settlementRepo.On("FindBySellerAndPeriod", uint64(1), mock.Anything, mock.Anything).Return(nil, gorm.ErrRecordNotFound)
		settlementRepo.On("GetPendingSettlementOrders", uint64(1), mock.Anything, mock.Anything).Return(orders, nil)
		settlementRepo.On("Create", mock.AnythingOfType("*domain.Settlement")).Return(nil)

		result, err := svc.CreateSettlement(1, periodStart, periodEnd)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, uint64(1), result.SellerID)
		// 총액: 50000
		// PG 수수료 (3.3%): 1650
		// 플랫폼 수수료 (5%): 2500
		// 정산액: 50000 - 1650 - 2500 = 45850
		assert.Equal(t, float64(50000), result.TotalSales)
	})

	t.Run("실패 - 정산 기간 오류", func(t *testing.T) {
		settlementRepo := new(MockSettlementRepository)
		orderRepo := new(MockOrderRepositoryForSettlement)

		svc := NewSettlementService(settlementRepo, orderRepo)

		periodStart := time.Now()
		periodEnd := time.Now().AddDate(0, -1, 0) // 시작일보다 이전

		result, err := svc.CreateSettlement(1, periodStart, periodEnd)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, ErrInvalidPeriod, err)
	})

	t.Run("실패 - 중복 정산", func(t *testing.T) {
		settlementRepo := new(MockSettlementRepository)
		orderRepo := new(MockOrderRepositoryForSettlement)

		svc := NewSettlementService(settlementRepo, orderRepo)

		periodStart := time.Now().AddDate(0, -1, 0)
		periodEnd := time.Now()

		existingSettlement := &domain.Settlement{
			ID:       1,
			SellerID: 1,
		}

		settlementRepo.On("FindBySellerAndPeriod", uint64(1), mock.Anything, mock.Anything).Return(existingSettlement, nil)

		result, err := svc.CreateSettlement(1, periodStart, periodEnd)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, ErrDuplicateSettlement, err)
	})

	t.Run("실패 - 정산 대상 주문 없음", func(t *testing.T) {
		settlementRepo := new(MockSettlementRepository)
		orderRepo := new(MockOrderRepositoryForSettlement)

		svc := NewSettlementService(settlementRepo, orderRepo)

		periodStart := time.Now().AddDate(0, -1, 0)
		periodEnd := time.Now()

		settlementRepo.On("FindBySellerAndPeriod", uint64(1), mock.Anything, mock.Anything).Return(nil, gorm.ErrRecordNotFound)
		settlementRepo.On("GetPendingSettlementOrders", uint64(1), mock.Anything, mock.Anything).Return([]*domain.OrderItem{}, nil)

		result, err := svc.CreateSettlement(1, periodStart, periodEnd)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, ErrSettlementNoOrders, err)
	})
}

func TestGetSettlement(t *testing.T) {
	t.Run("성공 - 정산 조회", func(t *testing.T) {
		settlementRepo := new(MockSettlementRepository)
		orderRepo := new(MockOrderRepositoryForSettlement)

		svc := NewSettlementService(settlementRepo, orderRepo)

		settlement := &domain.Settlement{
			ID:               1,
			SellerID:         1,
			TotalSales:       50000,
			PGFees:           1650,
			PlatformFees:     2500,
			SettlementAmount: 45850,
			Status:           domain.SettlementStatusPending,
			PeriodStart:      time.Now().AddDate(0, -1, 0),
			PeriodEnd:        time.Now(),
			CreatedAt:        time.Now(),
		}

		settlementRepo.On("FindByID", uint64(1)).Return(settlement, nil)

		result, err := svc.GetSettlement(1, 1)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, uint64(1), result.ID)
		assert.Equal(t, float64(50000), result.TotalSales)
		assert.Equal(t, float64(45850), result.SettlementAmount)
	})

	t.Run("실패 - 정산 없음", func(t *testing.T) {
		settlementRepo := new(MockSettlementRepository)
		orderRepo := new(MockOrderRepositoryForSettlement)

		svc := NewSettlementService(settlementRepo, orderRepo)

		settlementRepo.On("FindByID", uint64(999)).Return(nil, gorm.ErrRecordNotFound)

		result, err := svc.GetSettlement(999, 1)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, ErrSettlementNotFound, err)
	})

	t.Run("실패 - 다른 판매자의 정산", func(t *testing.T) {
		settlementRepo := new(MockSettlementRepository)
		orderRepo := new(MockOrderRepositoryForSettlement)

		svc := NewSettlementService(settlementRepo, orderRepo)

		settlement := &domain.Settlement{
			ID:       1,
			SellerID: 2, // 다른 판매자
		}

		settlementRepo.On("FindByID", uint64(1)).Return(settlement, nil)

		result, err := svc.GetSettlement(1, 1) // 판매자 ID: 1

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, ErrSettlementForbidden, err)
	})
}

func TestListSettlements(t *testing.T) {
	t.Run("성공 - 정산 목록 조회", func(t *testing.T) {
		settlementRepo := new(MockSettlementRepository)
		orderRepo := new(MockOrderRepositoryForSettlement)

		svc := NewSettlementService(settlementRepo, orderRepo)

		settlements := []*domain.Settlement{
			{
				ID:               1,
				SellerID:         1,
				TotalSales:       50000,
				SettlementAmount: 45850,
				Status:           domain.SettlementStatusCompleted,
			},
			{
				ID:               2,
				SellerID:         1,
				TotalSales:       30000,
				SettlementAmount: 27510,
				Status:           domain.SettlementStatusPending,
			},
		}

		req := &domain.SettlementListRequest{
			Page:  1,
			Limit: 10,
		}

		settlementRepo.On("ListBySeller", uint64(1), req).Return(settlements, int64(2), nil)

		result, total, err := svc.ListSettlements(1, req)

		assert.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Equal(t, int64(2), total)
	})
}

func TestGetSettlementSummary(t *testing.T) {
	t.Run("성공 - 정산 요약 조회", func(t *testing.T) {
		settlementRepo := new(MockSettlementRepository)
		orderRepo := new(MockOrderRepositoryForSettlement)

		svc := NewSettlementService(settlementRepo, orderRepo)

		summary := &domain.SettlementSummary{
			TotalSales:        100000,
			TotalRefunds:      10000,
			TotalPGFees:       2970,
			TotalPlatformFees: 4500,
			TotalSettled:      82530,
			PendingAmount:     0,
			Currency:          "KRW",
		}

		settlementRepo.On("GetSummaryBySeller", uint64(1)).Return(summary, nil)

		result, err := svc.GetSettlementSummary(1)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, float64(100000), result.TotalSales)
		assert.Equal(t, float64(82530), result.TotalSettled)
	})
}

func TestProcessSettlement(t *testing.T) {
	t.Run("성공 - 정산 처리", func(t *testing.T) {
		settlementRepo := new(MockSettlementRepository)
		orderRepo := new(MockOrderRepositoryForSettlement)

		svc := NewSettlementService(settlementRepo, orderRepo)

		settlement := &domain.Settlement{
			ID:       1,
			SellerID: 1,
			Status:   domain.SettlementStatusPending,
		}

		settlementRepo.On("FindByID", uint64(1)).Return(settlement, nil)
		settlementRepo.On("Update", uint64(1), mock.AnythingOfType("*domain.Settlement")).Return(nil)

		req := &domain.ProcessSettlementRequest{
			Notes: "정산 완료",
		}

		err := svc.ProcessSettlement(1, 100, req)

		assert.NoError(t, err)
	})

	t.Run("실패 - 이미 처리된 정산", func(t *testing.T) {
		settlementRepo := new(MockSettlementRepository)
		orderRepo := new(MockOrderRepositoryForSettlement)

		svc := NewSettlementService(settlementRepo, orderRepo)

		settlement := &domain.Settlement{
			ID:       1,
			SellerID: 1,
			Status:   domain.SettlementStatusCompleted, // 이미 완료됨
		}

		settlementRepo.On("FindByID", uint64(1)).Return(settlement, nil)

		req := &domain.ProcessSettlementRequest{
			Notes: "정산 완료",
		}

		err := svc.ProcessSettlement(1, 100, req)

		assert.Error(t, err)
		assert.Equal(t, ErrSettlementNotPending, err)
	})

	t.Run("실패 - 정산 없음", func(t *testing.T) {
		settlementRepo := new(MockSettlementRepository)
		orderRepo := new(MockOrderRepositoryForSettlement)

		svc := NewSettlementService(settlementRepo, orderRepo)

		settlementRepo.On("FindByID", uint64(999)).Return(nil, gorm.ErrRecordNotFound)

		req := &domain.ProcessSettlementRequest{
			Notes: "정산 완료",
		}

		err := svc.ProcessSettlement(999, 100, req)

		assert.Error(t, err)
		assert.Equal(t, ErrSettlementNotFound, err)
	})
}
