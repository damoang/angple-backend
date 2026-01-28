package service

import (
	"testing"

	"github.com/damoang/angple-backend/internal/plugins/commerce/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"gorm.io/gorm"
)

// MockCartRepository 장바구니 저장소 모의 객체
type MockCartRepository struct {
	mock.Mock
}

func (m *MockCartRepository) Create(cart *domain.Cart) error {
	args := m.Called(cart)
	return args.Error(0)
}

func (m *MockCartRepository) Update(id uint64, cart *domain.Cart) error {
	args := m.Called(id, cart)
	return args.Error(0)
}

func (m *MockCartRepository) Delete(id uint64) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *MockCartRepository) DeleteByUserID(userID uint64) error {
	args := m.Called(userID)
	return args.Error(0)
}

func (m *MockCartRepository) FindByID(id uint64) (*domain.Cart, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Cart), args.Error(1)
}

func (m *MockCartRepository) FindByUserAndProduct(userID, productID uint64) (*domain.Cart, error) {
	args := m.Called(userID, productID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Cart), args.Error(1)
}

func (m *MockCartRepository) ListByUser(userID uint64) ([]*domain.Cart, error) {
	args := m.Called(userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.Cart), args.Error(1)
}

func (m *MockCartRepository) ListByUserWithProducts(userID uint64) ([]*domain.Cart, error) {
	args := m.Called(userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.Cart), args.Error(1)
}

func (m *MockCartRepository) IncrementQuantity(id uint64, quantity int) error {
	args := m.Called(id, quantity)
	return args.Error(0)
}

func (m *MockCartRepository) SetQuantity(id uint64, quantity int) error {
	args := m.Called(id, quantity)
	return args.Error(0)
}

func TestGetCart(t *testing.T) {
	t.Run("성공 - 장바구니 조회", func(t *testing.T) {
		cartRepo := new(MockCartRepository)
		productRepo := new(MockProductRepository)
		svc := NewCartService(cartRepo, productRepo)

		carts := []*domain.Cart{
			{
				ID:        1,
				UserID:    1,
				ProductID: 1,
				Quantity:  2,
				Product: &domain.Product{
					ID:          1,
					Name:        "테스트 상품",
					Price:       10000,
					ProductType: domain.ProductTypeDigital,
					Currency:    "KRW",
				},
			},
		}

		cartRepo.On("ListByUserWithProducts", uint64(1)).Return(carts, nil)

		cart, err := svc.GetCart(1)

		assert.NoError(t, err)
		assert.NotNil(t, cart)
		assert.Equal(t, 1, cart.ItemCount)
		assert.Equal(t, 2, cart.TotalCount)
		assert.Equal(t, float64(20000), cart.Subtotal)
		assert.Equal(t, "KRW", cart.Currency)
		cartRepo.AssertExpectations(t)
	})

	t.Run("성공 - 빈 장바구니", func(t *testing.T) {
		cartRepo := new(MockCartRepository)
		productRepo := new(MockProductRepository)
		svc := NewCartService(cartRepo, productRepo)

		cartRepo.On("ListByUserWithProducts", uint64(1)).Return([]*domain.Cart{}, nil)

		cart, err := svc.GetCart(1)

		assert.NoError(t, err)
		assert.NotNil(t, cart)
		assert.Equal(t, 0, cart.ItemCount)
		assert.Equal(t, float64(0), cart.Subtotal)
		cartRepo.AssertExpectations(t)
	})
}

func TestAddToCart(t *testing.T) {
	t.Run("성공 - 새 상품 추가", func(t *testing.T) {
		cartRepo := new(MockCartRepository)
		productRepo := new(MockProductRepository)
		svc := NewCartService(cartRepo, productRepo)

		product := &domain.Product{
			ID:          1,
			Name:        "테스트 상품",
			Price:       10000,
			ProductType: domain.ProductTypeDigital,
			Status:      domain.ProductStatusPublished,
			Currency:    "KRW",
		}

		productRepo.On("FindByID", uint64(1)).Return(product, nil)
		cartRepo.On("FindByUserAndProduct", uint64(1), uint64(1)).Return(nil, gorm.ErrRecordNotFound)
		cartRepo.On("Create", mock.AnythingOfType("*domain.Cart")).Return(nil)

		req := &domain.AddToCartRequest{
			ProductID: 1,
			Quantity:  2,
		}
		item, err := svc.AddToCart(1, req)

		assert.NoError(t, err)
		assert.NotNil(t, item)
		assert.Equal(t, 2, item.Quantity)
		assert.Equal(t, float64(20000), item.Subtotal)
		productRepo.AssertExpectations(t)
		cartRepo.AssertExpectations(t)
	})

	t.Run("성공 - 기존 상품 수량 증가", func(t *testing.T) {
		cartRepo := new(MockCartRepository)
		productRepo := new(MockProductRepository)
		svc := NewCartService(cartRepo, productRepo)

		product := &domain.Product{
			ID:          1,
			Name:        "테스트 상품",
			Price:       10000,
			ProductType: domain.ProductTypeDigital,
			Status:      domain.ProductStatusPublished,
			Currency:    "KRW",
		}

		existingCart := &domain.Cart{
			ID:        1,
			UserID:    1,
			ProductID: 1,
			Quantity:  2,
		}

		productRepo.On("FindByID", uint64(1)).Return(product, nil)
		cartRepo.On("FindByUserAndProduct", uint64(1), uint64(1)).Return(existingCart, nil)
		cartRepo.On("SetQuantity", uint64(1), 5).Return(nil)

		req := &domain.AddToCartRequest{
			ProductID: 1,
			Quantity:  3,
		}
		item, err := svc.AddToCart(1, req)

		assert.NoError(t, err)
		assert.NotNil(t, item)
		assert.Equal(t, 5, item.Quantity) // 기존 2 + 새로 추가 3
		productRepo.AssertExpectations(t)
		cartRepo.AssertExpectations(t)
	})

	t.Run("실패 - 상품 없음", func(t *testing.T) {
		cartRepo := new(MockCartRepository)
		productRepo := new(MockProductRepository)
		svc := NewCartService(cartRepo, productRepo)

		productRepo.On("FindByID", uint64(1)).Return(nil, gorm.ErrRecordNotFound)

		req := &domain.AddToCartRequest{
			ProductID: 1,
			Quantity:  1,
		}
		item, err := svc.AddToCart(1, req)

		assert.Error(t, err)
		assert.Nil(t, item)
		assert.Equal(t, ErrProductNotFound, err)
		productRepo.AssertExpectations(t)
	})

	t.Run("실패 - 비공개 상품", func(t *testing.T) {
		cartRepo := new(MockCartRepository)
		productRepo := new(MockProductRepository)
		svc := NewCartService(cartRepo, productRepo)

		product := &domain.Product{
			ID:          1,
			Name:        "테스트 상품",
			Price:       10000,
			ProductType: domain.ProductTypeDigital,
			Status:      domain.ProductStatusDraft, // 비공개
		}

		productRepo.On("FindByID", uint64(1)).Return(product, nil)

		req := &domain.AddToCartRequest{
			ProductID: 1,
			Quantity:  1,
		}
		item, err := svc.AddToCart(1, req)

		assert.Error(t, err)
		assert.Nil(t, item)
		assert.Equal(t, ErrProductNotAvailable, err)
		productRepo.AssertExpectations(t)
	})

	t.Run("실패 - 재고 부족", func(t *testing.T) {
		cartRepo := new(MockCartRepository)
		productRepo := new(MockProductRepository)
		svc := NewCartService(cartRepo, productRepo)

		stock := 5
		product := &domain.Product{
			ID:            1,
			Name:          "실물 상품",
			Price:         10000,
			ProductType:   domain.ProductTypePhysical,
			Status:        domain.ProductStatusPublished,
			StockQuantity: &stock,
		}

		productRepo.On("FindByID", uint64(1)).Return(product, nil)

		req := &domain.AddToCartRequest{
			ProductID: 1,
			Quantity:  10, // 재고보다 많은 수량
		}
		item, err := svc.AddToCart(1, req)

		assert.Error(t, err)
		assert.Nil(t, item)
		assert.Equal(t, ErrInsufficientStock, err)
		productRepo.AssertExpectations(t)
	})
}

func TestRemoveFromCart(t *testing.T) {
	t.Run("성공 - 장바구니 아이템 삭제", func(t *testing.T) {
		cartRepo := new(MockCartRepository)
		productRepo := new(MockProductRepository)
		svc := NewCartService(cartRepo, productRepo)

		cart := &domain.Cart{
			ID:        1,
			UserID:    1,
			ProductID: 1,
			Quantity:  2,
		}

		cartRepo.On("FindByID", uint64(1)).Return(cart, nil)
		cartRepo.On("Delete", uint64(1)).Return(nil)

		err := svc.RemoveFromCart(1, 1)

		assert.NoError(t, err)
		cartRepo.AssertExpectations(t)
	})

	t.Run("실패 - 아이템 없음", func(t *testing.T) {
		cartRepo := new(MockCartRepository)
		productRepo := new(MockProductRepository)
		svc := NewCartService(cartRepo, productRepo)

		cartRepo.On("FindByID", uint64(1)).Return(nil, gorm.ErrRecordNotFound)

		err := svc.RemoveFromCart(1, 1)

		assert.Error(t, err)
		assert.Equal(t, ErrCartItemNotFound, err)
		cartRepo.AssertExpectations(t)
	})

	t.Run("실패 - 다른 사용자의 아이템", func(t *testing.T) {
		cartRepo := new(MockCartRepository)
		productRepo := new(MockProductRepository)
		svc := NewCartService(cartRepo, productRepo)

		cart := &domain.Cart{
			ID:        1,
			UserID:    2, // 다른 사용자
			ProductID: 1,
			Quantity:  2,
		}

		cartRepo.On("FindByID", uint64(1)).Return(cart, nil)

		err := svc.RemoveFromCart(1, 1)

		assert.Error(t, err)
		assert.Equal(t, ErrCartItemForbidden, err)
		cartRepo.AssertExpectations(t)
	})
}

func TestClearCart(t *testing.T) {
	t.Run("성공 - 장바구니 비우기", func(t *testing.T) {
		cartRepo := new(MockCartRepository)
		productRepo := new(MockProductRepository)
		svc := NewCartService(cartRepo, productRepo)

		cartRepo.On("DeleteByUserID", uint64(1)).Return(nil)

		err := svc.ClearCart(1)

		assert.NoError(t, err)
		cartRepo.AssertExpectations(t)
	})
}
