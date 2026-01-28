package service

import (
	"testing"
	"time"

	"github.com/damoang/angple-backend/internal/plugins/commerce/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"gorm.io/gorm"
)

// MockCouponRepository 쿠폰 저장소 목
type MockCouponRepository struct {
	mock.Mock
}

func (m *MockCouponRepository) Create(coupon *domain.Coupon) error {
	args := m.Called(coupon)
	return args.Error(0)
}

func (m *MockCouponRepository) Update(id uint64, coupon *domain.Coupon) error {
	args := m.Called(id, coupon)
	return args.Error(0)
}

func (m *MockCouponRepository) Delete(id uint64) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *MockCouponRepository) FindByID(id uint64) (*domain.Coupon, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Coupon), args.Error(1)
}

func (m *MockCouponRepository) FindByCode(code string) (*domain.Coupon, error) {
	args := m.Called(code)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Coupon), args.Error(1)
}

func (m *MockCouponRepository) List(req *domain.CouponListRequest) ([]*domain.Coupon, int64, error) {
	args := m.Called(req)
	return args.Get(0).([]*domain.Coupon), args.Get(1).(int64), args.Error(2)
}

func (m *MockCouponRepository) ListPublicActive() ([]*domain.Coupon, error) {
	args := m.Called()
	return args.Get(0).([]*domain.Coupon), args.Error(1)
}

func (m *MockCouponRepository) IncrementUsageCount(id uint64) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *MockCouponRepository) DecrementUsageCount(id uint64) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *MockCouponRepository) UpdateExpiredCoupons() (int64, error) {
	args := m.Called()
	return args.Get(0).(int64), args.Error(1)
}

// MockCouponUsageRepository 쿠폰 사용 내역 저장소 목
type MockCouponUsageRepository struct {
	mock.Mock
}

func (m *MockCouponUsageRepository) Create(usage *domain.CouponUsage) error {
	args := m.Called(usage)
	return args.Error(0)
}

func (m *MockCouponUsageRepository) FindByOrderID(orderID uint64) (*domain.CouponUsage, error) {
	args := m.Called(orderID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.CouponUsage), args.Error(1)
}

func (m *MockCouponUsageRepository) CountByUserAndCoupon(userID, couponID uint64) (int64, error) {
	args := m.Called(userID, couponID)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockCouponUsageRepository) ListByUserID(userID uint64) ([]*domain.CouponUsage, error) {
	args := m.Called(userID)
	return args.Get(0).([]*domain.CouponUsage), args.Error(1)
}

func (m *MockCouponUsageRepository) DeleteByOrderID(orderID uint64) error {
	args := m.Called(orderID)
	return args.Error(0)
}

// MockOrderRepositoryForCoupon 주문 저장소 목 (쿠폰용)
type MockOrderRepositoryForCoupon struct {
	mock.Mock
}

func (m *MockOrderRepositoryForCoupon) Create(order *domain.Order) error {
	args := m.Called(order)
	return args.Error(0)
}

func (m *MockOrderRepositoryForCoupon) CreateWithItems(order *domain.Order, items []domain.OrderItem) error {
	args := m.Called(order, items)
	return args.Error(0)
}

func (m *MockOrderRepositoryForCoupon) Update(id uint64, order *domain.Order) error {
	args := m.Called(id, order)
	return args.Error(0)
}

func (m *MockOrderRepositoryForCoupon) UpdateStatus(id uint64, status domain.OrderStatus) error {
	args := m.Called(id, status)
	return args.Error(0)
}

func (m *MockOrderRepositoryForCoupon) FindByID(id uint64) (*domain.Order, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Order), args.Error(1)
}

func (m *MockOrderRepositoryForCoupon) FindByIDWithItems(id uint64) (*domain.Order, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Order), args.Error(1)
}

func (m *MockOrderRepositoryForCoupon) FindByOrderNumber(orderNumber string) (*domain.Order, error) {
	args := m.Called(orderNumber)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Order), args.Error(1)
}

func (m *MockOrderRepositoryForCoupon) FindByOrderNumberWithItems(orderNumber string) (*domain.Order, error) {
	args := m.Called(orderNumber)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Order), args.Error(1)
}

func (m *MockOrderRepositoryForCoupon) ListByUser(userID uint64, req *domain.OrderListRequest) ([]*domain.Order, int64, error) {
	args := m.Called(userID, req)
	return args.Get(0).([]*domain.Order), args.Get(1).(int64), args.Error(2)
}

func (m *MockOrderRepositoryForCoupon) ListByUserWithItems(userID uint64, req *domain.OrderListRequest) ([]*domain.Order, int64, error) {
	args := m.Called(userID, req)
	return args.Get(0).([]*domain.Order), args.Get(1).(int64), args.Error(2)
}

func (m *MockOrderRepositoryForCoupon) ListBySeller(sellerID uint64, req *domain.OrderListRequest) ([]*domain.Order, int64, error) {
	args := m.Called(sellerID, req)
	return args.Get(0).([]*domain.Order), args.Get(1).(int64), args.Error(2)
}

func (m *MockOrderRepositoryForCoupon) FindItemByID(itemID uint64) (*domain.OrderItem, error) {
	args := m.Called(itemID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.OrderItem), args.Error(1)
}

func (m *MockOrderRepositoryForCoupon) UpdateItemStatus(itemID uint64, status domain.OrderItemStatus) error {
	args := m.Called(itemID, status)
	return args.Error(0)
}

func (m *MockOrderRepositoryForCoupon) GenerateOrderNumber() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func TestCreateCoupon(t *testing.T) {
	t.Run("성공 - 정액 할인 쿠폰 생성", func(t *testing.T) {
		couponRepo := new(MockCouponRepository)
		couponUsageRepo := new(MockCouponUsageRepository)
		orderRepo := new(MockOrderRepositoryForCoupon)

		svc := NewCouponService(couponRepo, couponUsageRepo, orderRepo)

		couponRepo.On("FindByCode", "WELCOME10").Return(nil, gorm.ErrRecordNotFound)
		couponRepo.On("Create", mock.AnythingOfType("*domain.Coupon")).Return(nil)

		req := &domain.CreateCouponRequest{
			Code:           "welcome10",
			Name:           "신규 회원 할인",
			DiscountType:   "fixed",
			DiscountValue:  5000,
			MinOrderAmount: 10000,
			UsagePerUser:   1,
			IsPublic:       true,
		}

		result, err := svc.CreateCoupon(1, req)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "WELCOME10", result.Code)
		assert.Equal(t, "fixed", result.DiscountType)
		assert.Equal(t, float64(5000), result.DiscountValue)
	})

	t.Run("성공 - 정률 할인 쿠폰 생성", func(t *testing.T) {
		couponRepo := new(MockCouponRepository)
		couponUsageRepo := new(MockCouponUsageRepository)
		orderRepo := new(MockOrderRepositoryForCoupon)

		svc := NewCouponService(couponRepo, couponUsageRepo, orderRepo)

		couponRepo.On("FindByCode", "SUMMER20").Return(nil, gorm.ErrRecordNotFound)
		couponRepo.On("Create", mock.AnythingOfType("*domain.Coupon")).Return(nil)

		maxDiscount := float64(10000)
		req := &domain.CreateCouponRequest{
			Code:          "summer20",
			Name:          "여름 세일 20%",
			DiscountType:  "percent",
			DiscountValue: 20,
			MaxDiscount:   &maxDiscount,
			IsPublic:      true,
		}

		result, err := svc.CreateCoupon(1, req)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "percent", result.DiscountType)
		assert.Equal(t, float64(20), result.DiscountValue)
		assert.Equal(t, float64(10000), *result.MaxDiscount)
	})

	t.Run("실패 - 중복 코드", func(t *testing.T) {
		couponRepo := new(MockCouponRepository)
		couponUsageRepo := new(MockCouponUsageRepository)
		orderRepo := new(MockOrderRepositoryForCoupon)

		svc := NewCouponService(couponRepo, couponUsageRepo, orderRepo)

		existingCoupon := &domain.Coupon{
			ID:   1,
			Code: "EXISTING",
		}
		couponRepo.On("FindByCode", "EXISTING").Return(existingCoupon, nil)

		req := &domain.CreateCouponRequest{
			Code:          "existing",
			Name:          "중복 쿠폰",
			DiscountType:  "fixed",
			DiscountValue: 1000,
		}

		result, err := svc.CreateCoupon(1, req)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, ErrCouponCodeExists, err)
	})
}

func TestValidateCoupon(t *testing.T) {
	t.Run("성공 - 유효한 쿠폰", func(t *testing.T) {
		couponRepo := new(MockCouponRepository)
		couponUsageRepo := new(MockCouponUsageRepository)
		orderRepo := new(MockOrderRepositoryForCoupon)

		svc := NewCouponService(couponRepo, couponUsageRepo, orderRepo)

		coupon := &domain.Coupon{
			ID:             1,
			Code:           "VALID10",
			Name:           "유효 쿠폰",
			DiscountType:   domain.DiscountTypeFixed,
			DiscountValue:  5000,
			MinOrderAmount: 10000,
			UsagePerUser:   1,
			Status:         domain.CouponStatusActive,
		}

		couponRepo.On("FindByCode", "VALID10").Return(coupon, nil)
		couponUsageRepo.On("CountByUserAndCoupon", uint64(1), uint64(1)).Return(int64(0), nil)

		result, err := svc.ValidateCoupon(1, "valid10", 20000)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.Valid)
		assert.Equal(t, "VALID10", result.Code)
	})

	t.Run("실패 - 만료된 쿠폰", func(t *testing.T) {
		couponRepo := new(MockCouponRepository)
		couponUsageRepo := new(MockCouponUsageRepository)
		orderRepo := new(MockOrderRepositoryForCoupon)

		svc := NewCouponService(couponRepo, couponUsageRepo, orderRepo)

		expiredTime := time.Now().Add(-24 * time.Hour)
		coupon := &domain.Coupon{
			ID:            1,
			Code:          "EXPIRED",
			Name:          "만료 쿠폰",
			DiscountType:  domain.DiscountTypeFixed,
			DiscountValue: 5000,
			Status:        domain.CouponStatusActive,
			ExpiresAt:     &expiredTime,
		}

		couponRepo.On("FindByCode", "EXPIRED").Return(coupon, nil)

		result, err := svc.ValidateCoupon(1, "expired", 20000)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.Valid)
		assert.Contains(t, result.Message, "만료")
	})

	t.Run("실패 - 비활성 쿠폰", func(t *testing.T) {
		couponRepo := new(MockCouponRepository)
		couponUsageRepo := new(MockCouponUsageRepository)
		orderRepo := new(MockOrderRepositoryForCoupon)

		svc := NewCouponService(couponRepo, couponUsageRepo, orderRepo)

		coupon := &domain.Coupon{
			ID:            1,
			Code:          "INACTIVE",
			Name:          "비활성 쿠폰",
			DiscountType:  domain.DiscountTypeFixed,
			DiscountValue: 5000,
			Status:        domain.CouponStatusInactive,
		}

		couponRepo.On("FindByCode", "INACTIVE").Return(coupon, nil)

		result, err := svc.ValidateCoupon(1, "inactive", 20000)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.Valid)
		assert.Contains(t, result.Message, "비활성")
	})

	t.Run("실패 - 사용자별 사용 제한 초과", func(t *testing.T) {
		couponRepo := new(MockCouponRepository)
		couponUsageRepo := new(MockCouponUsageRepository)
		orderRepo := new(MockOrderRepositoryForCoupon)

		svc := NewCouponService(couponRepo, couponUsageRepo, orderRepo)

		coupon := &domain.Coupon{
			ID:            1,
			Code:          "ONCE",
			Name:          "1회 쿠폰",
			DiscountType:  domain.DiscountTypeFixed,
			DiscountValue: 5000,
			UsagePerUser:  1,
			Status:        domain.CouponStatusActive,
		}

		couponRepo.On("FindByCode", "ONCE").Return(coupon, nil)
		couponUsageRepo.On("CountByUserAndCoupon", uint64(1), uint64(1)).Return(int64(1), nil) // 이미 1회 사용

		result, err := svc.ValidateCoupon(1, "once", 20000)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.Valid)
		assert.Contains(t, result.Message, "최대 사용 횟수")
	})

	t.Run("실패 - 최소 주문 금액 미달", func(t *testing.T) {
		couponRepo := new(MockCouponRepository)
		couponUsageRepo := new(MockCouponUsageRepository)
		orderRepo := new(MockOrderRepositoryForCoupon)

		svc := NewCouponService(couponRepo, couponUsageRepo, orderRepo)

		coupon := &domain.Coupon{
			ID:             1,
			Code:           "MIN50K",
			Name:           "5만원 이상 쿠폰",
			DiscountType:   domain.DiscountTypeFixed,
			DiscountValue:  5000,
			MinOrderAmount: 50000,
			UsagePerUser:   1,
			Status:         domain.CouponStatusActive,
		}

		couponRepo.On("FindByCode", "MIN50K").Return(coupon, nil)
		couponUsageRepo.On("CountByUserAndCoupon", uint64(1), uint64(1)).Return(int64(0), nil)

		result, err := svc.ValidateCoupon(1, "min50k", 30000) // 30000원 주문

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.Valid)
		assert.Contains(t, result.Message, "최소 주문 금액")
	})

	t.Run("실패 - 존재하지 않는 쿠폰", func(t *testing.T) {
		couponRepo := new(MockCouponRepository)
		couponUsageRepo := new(MockCouponUsageRepository)
		orderRepo := new(MockOrderRepositoryForCoupon)

		svc := NewCouponService(couponRepo, couponUsageRepo, orderRepo)

		couponRepo.On("FindByCode", "NOTEXIST").Return(nil, gorm.ErrRecordNotFound)

		result, err := svc.ValidateCoupon(1, "notexist", 20000)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.Valid)
		assert.Contains(t, result.Message, "찾을 수 없")
	})
}

func TestApplyCoupon(t *testing.T) {
	t.Run("성공 - 쿠폰 적용", func(t *testing.T) {
		couponRepo := new(MockCouponRepository)
		couponUsageRepo := new(MockCouponUsageRepository)
		orderRepo := new(MockOrderRepositoryForCoupon)

		svc := NewCouponService(couponRepo, couponUsageRepo, orderRepo)

		order := &domain.Order{
			ID:       1,
			UserID:   1,
			Subtotal: 50000,
			Total:    50000,
			Status:   domain.OrderStatusPending,
		}

		coupon := &domain.Coupon{
			ID:             1,
			Code:           "APPLY10",
			Name:           "적용 쿠폰",
			DiscountType:   domain.DiscountTypeFixed,
			DiscountValue:  5000,
			MinOrderAmount: 10000,
			UsagePerUser:   1,
			Status:         domain.CouponStatusActive,
		}

		orderRepo.On("FindByID", uint64(1)).Return(order, nil)
		couponUsageRepo.On("FindByOrderID", uint64(1)).Return(nil, gorm.ErrRecordNotFound)
		couponRepo.On("FindByCode", "APPLY10").Return(coupon, nil)
		couponUsageRepo.On("CountByUserAndCoupon", uint64(1), uint64(1)).Return(int64(0), nil)
		couponUsageRepo.On("Create", mock.AnythingOfType("*domain.CouponUsage")).Return(nil)
		couponRepo.On("IncrementUsageCount", uint64(1)).Return(nil)
		orderRepo.On("Update", uint64(1), mock.AnythingOfType("*domain.Order")).Return(nil)

		discount, err := svc.ApplyCoupon(1, 1, "apply10")

		assert.NoError(t, err)
		assert.Equal(t, float64(5000), discount)
	})

	t.Run("실패 - 주문 없음", func(t *testing.T) {
		couponRepo := new(MockCouponRepository)
		couponUsageRepo := new(MockCouponUsageRepository)
		orderRepo := new(MockOrderRepositoryForCoupon)

		svc := NewCouponService(couponRepo, couponUsageRepo, orderRepo)

		orderRepo.On("FindByID", uint64(999)).Return(nil, gorm.ErrRecordNotFound)

		discount, err := svc.ApplyCoupon(1, 999, "apply10")

		assert.Error(t, err)
		assert.Equal(t, float64(0), discount)
		assert.Equal(t, ErrOrderNotFound, err)
	})

	t.Run("실패 - 다른 사용자의 주문", func(t *testing.T) {
		couponRepo := new(MockCouponRepository)
		couponUsageRepo := new(MockCouponUsageRepository)
		orderRepo := new(MockOrderRepositoryForCoupon)

		svc := NewCouponService(couponRepo, couponUsageRepo, orderRepo)

		order := &domain.Order{
			ID:       1,
			UserID:   2, // 다른 사용자
			Subtotal: 50000,
			Total:    50000,
			Status:   domain.OrderStatusPending,
		}

		orderRepo.On("FindByID", uint64(1)).Return(order, nil)

		discount, err := svc.ApplyCoupon(1, 1, "apply10") // 사용자 ID: 1

		assert.Error(t, err)
		assert.Equal(t, float64(0), discount)
		assert.Equal(t, ErrOrderForbidden, err)
	})

	t.Run("실패 - 이미 쿠폰 적용됨", func(t *testing.T) {
		couponRepo := new(MockCouponRepository)
		couponUsageRepo := new(MockCouponUsageRepository)
		orderRepo := new(MockOrderRepositoryForCoupon)

		svc := NewCouponService(couponRepo, couponUsageRepo, orderRepo)

		order := &domain.Order{
			ID:       1,
			UserID:   1,
			Subtotal: 50000,
			Total:    45000,
			Discount: 5000,
			Status:   domain.OrderStatusPending,
		}

		existingUsage := &domain.CouponUsage{
			ID:       1,
			CouponID: 1,
			OrderID:  1,
		}

		orderRepo.On("FindByID", uint64(1)).Return(order, nil)
		couponUsageRepo.On("FindByOrderID", uint64(1)).Return(existingUsage, nil)

		discount, err := svc.ApplyCoupon(1, 1, "apply10")

		assert.Error(t, err)
		assert.Equal(t, float64(0), discount)
		assert.Equal(t, ErrCouponAlreadyApplied, err)
	})
}

func TestGetPublicCoupons(t *testing.T) {
	t.Run("성공 - 공개 쿠폰 목록 조회", func(t *testing.T) {
		couponRepo := new(MockCouponRepository)
		couponUsageRepo := new(MockCouponUsageRepository)
		orderRepo := new(MockOrderRepositoryForCoupon)

		svc := NewCouponService(couponRepo, couponUsageRepo, orderRepo)

		coupons := []*domain.Coupon{
			{
				ID:            1,
				Code:          "PUBLIC1",
				Name:          "공개 쿠폰 1",
				DiscountType:  domain.DiscountTypeFixed,
				DiscountValue: 5000,
				Status:        domain.CouponStatusActive,
				IsPublic:      true,
			},
			{
				ID:            2,
				Code:          "PUBLIC2",
				Name:          "공개 쿠폰 2",
				DiscountType:  domain.DiscountTypePercent,
				DiscountValue: 10,
				Status:        domain.CouponStatusActive,
				IsPublic:      true,
			},
		}

		couponRepo.On("ListPublicActive").Return(coupons, nil)

		result, err := svc.GetPublicCoupons()

		assert.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Equal(t, "PUBLIC1", result[0].Code)
		assert.Equal(t, "PUBLIC2", result[1].Code)
	})
}

func TestCalculateDiscount(t *testing.T) {
	couponRepo := new(MockCouponRepository)
	couponUsageRepo := new(MockCouponUsageRepository)
	orderRepo := new(MockOrderRepositoryForCoupon)

	svc := NewCouponService(couponRepo, couponUsageRepo, orderRepo)

	t.Run("정액 할인", func(t *testing.T) {
		coupon := &domain.Coupon{
			DiscountType:   domain.DiscountTypeFixed,
			DiscountValue:  5000,
			MinOrderAmount: 10000,
		}

		discount := svc.CalculateDiscount(coupon, 50000)
		assert.Equal(t, float64(5000), discount)
	})

	t.Run("정률 할인", func(t *testing.T) {
		coupon := &domain.Coupon{
			DiscountType:   domain.DiscountTypePercent,
			DiscountValue:  10,
			MinOrderAmount: 10000,
		}

		discount := svc.CalculateDiscount(coupon, 50000)
		assert.Equal(t, float64(5000), discount) // 10% of 50000
	})

	t.Run("정률 할인 - 최대 할인 금액 제한", func(t *testing.T) {
		maxDiscount := float64(3000)
		coupon := &domain.Coupon{
			DiscountType:   domain.DiscountTypePercent,
			DiscountValue:  10,
			MaxDiscount:    &maxDiscount,
			MinOrderAmount: 10000,
		}

		discount := svc.CalculateDiscount(coupon, 50000)
		assert.Equal(t, float64(3000), discount) // capped at 3000
	})

	t.Run("최소 주문 금액 미달", func(t *testing.T) {
		coupon := &domain.Coupon{
			DiscountType:   domain.DiscountTypeFixed,
			DiscountValue:  5000,
			MinOrderAmount: 30000,
		}

		discount := svc.CalculateDiscount(coupon, 20000) // 최소 금액 미달
		assert.Equal(t, float64(0), discount)
	})

	t.Run("할인 금액이 주문 금액 초과 방지", func(t *testing.T) {
		coupon := &domain.Coupon{
			DiscountType:   domain.DiscountTypeFixed,
			DiscountValue:  10000,
			MinOrderAmount: 0,
		}

		discount := svc.CalculateDiscount(coupon, 5000) // 주문 금액보다 할인이 큼
		assert.Equal(t, float64(5000), discount)        // 주문 금액으로 제한
	})
}
