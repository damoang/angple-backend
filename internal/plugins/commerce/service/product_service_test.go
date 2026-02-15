package service

import (
	"testing"

	"github.com/damoang/angple-backend/internal/plugins/commerce/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockProductRepository 목 저장소
type MockProductRepository struct {
	mock.Mock
}

func (m *MockProductRepository) Create(product *domain.Product) error {
	args := m.Called(product)
	return args.Error(0)
}

func (m *MockProductRepository) Update(id uint64, product *domain.Product) error {
	args := m.Called(id, product)
	return args.Error(0)
}

func (m *MockProductRepository) Delete(id uint64) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *MockProductRepository) FindByID(id uint64) (*domain.Product, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Product), args.Error(1)
}

func (m *MockProductRepository) FindBySlug(slug string) (*domain.Product, error) {
	args := m.Called(slug)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Product), args.Error(1)
}

func (m *MockProductRepository) FindByIDWithDeleted(id uint64) (*domain.Product, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Product), args.Error(1)
}

func (m *MockProductRepository) ListBySeller(sellerID uint64, req *domain.ProductListRequest) ([]*domain.Product, int64, error) {
	args := m.Called(sellerID, req)
	return args.Get(0).([]*domain.Product), args.Get(1).(int64), args.Error(2)
}

func (m *MockProductRepository) ListPublished(req *domain.ProductListRequest) ([]*domain.Product, int64, error) {
	args := m.Called(req)
	return args.Get(0).([]*domain.Product), args.Get(1).(int64), args.Error(2)
}

func (m *MockProductRepository) IncrementViewCount(id uint64) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *MockProductRepository) IncrementSalesCount(id uint64, quantity int) error {
	args := m.Called(id, quantity)
	return args.Error(0)
}

func (m *MockProductRepository) IsSlugAvailable(slug string, excludeID uint64) (bool, error) {
	args := m.Called(slug, excludeID)
	return args.Bool(0), args.Error(1)
}

func TestCreateProduct(t *testing.T) {
	mockRepo := new(MockProductRepository)
	svc := NewProductService(mockRepo)

	t.Run("성공 - 디지털 상품 생성", func(t *testing.T) {
		req := &domain.CreateProductRequest{
			Name:        "테스트 디지털 상품",
			ProductType: "digital",
			Price:       10000,
		}

		mockRepo.On("IsSlugAvailable", mock.Anything, uint64(0)).Return(true, nil).Once()
		mockRepo.On("Create", mock.AnythingOfType("*domain.Product")).Return(nil).Once()

		result, err := svc.CreateProduct(1, req)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "테스트 디지털 상품", result.Name)
		assert.Equal(t, "digital", result.ProductType)
		assert.Equal(t, float64(10000), result.Price)
	})

	t.Run("성공 - 실물 상품 생성", func(t *testing.T) {
		stockQty := 100
		req := &domain.CreateProductRequest{
			Name:          "테스트 실물 상품",
			ProductType:   "physical",
			Price:         25000,
			StockQuantity: &stockQty,
		}

		mockRepo.On("IsSlugAvailable", mock.Anything, uint64(0)).Return(true, nil).Once()
		mockRepo.On("Create", mock.AnythingOfType("*domain.Product")).Return(nil).Once()

		result, err := svc.CreateProduct(1, req)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "테스트 실물 상품", result.Name)
		assert.Equal(t, "physical", result.ProductType)
	})

	t.Run("실패 - 슬러그 중복", func(t *testing.T) {
		// 새로운 목 인스턴스 생성
		mockRepo2 := new(MockProductRepository)
		svc2 := NewProductService(mockRepo2)

		req := &domain.CreateProductRequest{
			Name:        "테스트 상품",
			Slug:        "existing-slug",
			ProductType: "digital",
			Price:       10000,
		}

		mockRepo2.On("IsSlugAvailable", "existing-slug", uint64(0)).Return(false, nil).Once()

		result, err := svc2.CreateProduct(1, req)

		assert.Error(t, err)
		assert.Equal(t, ErrSlugAlreadyExists, err)
		assert.Nil(t, result)
	})
}

func TestGetMyProduct(t *testing.T) {
	mockRepo := new(MockProductRepository)
	svc := NewProductService(mockRepo)

	t.Run("성공 - 내 상품 조회", func(t *testing.T) {
		product := &domain.Product{
			ID:          1,
			SellerID:    1,
			Name:        "테스트 상품",
			ProductType: domain.ProductTypeDigital,
			Price:       10000,
		}

		mockRepo.On("FindByID", uint64(1)).Return(product, nil).Once()

		result, err := svc.GetMyProduct(1, 1)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, uint64(1), result.ID)
	})

	t.Run("실패 - 상품 없음", func(t *testing.T) {
		mockRepo.On("FindByID", uint64(999)).Return(nil, ErrProductNotFound).Once()

		result, err := svc.GetMyProduct(1, 999)

		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("실패 - 다른 판매자 상품", func(t *testing.T) {
		product := &domain.Product{
			ID:       1,
			SellerID: 2, // 다른 판매자
			Name:     "다른 판매자 상품",
		}

		mockRepo.On("FindByID", uint64(1)).Return(product, nil).Once()

		result, err := svc.GetMyProduct(1, 1)

		assert.Error(t, err)
		assert.Equal(t, ErrProductForbidden, err)
		assert.Nil(t, result)
	})
}

func TestDeleteProduct(t *testing.T) {
	mockRepo := new(MockProductRepository)
	svc := NewProductService(mockRepo)

	t.Run("성공 - 상품 삭제", func(t *testing.T) {
		product := &domain.Product{
			ID:       1,
			SellerID: 1,
			Name:     "삭제할 상품",
		}

		mockRepo.On("FindByID", uint64(1)).Return(product, nil).Once()
		mockRepo.On("Delete", uint64(1)).Return(nil).Once()

		err := svc.DeleteProduct(1, 1)

		assert.NoError(t, err)
	})

	t.Run("실패 - 다른 판매자 상품 삭제 시도", func(t *testing.T) {
		product := &domain.Product{
			ID:       1,
			SellerID: 2, // 다른 판매자
			Name:     "다른 판매자 상품",
		}

		mockRepo.On("FindByID", uint64(1)).Return(product, nil).Once()

		err := svc.DeleteProduct(1, 1)

		assert.Error(t, err)
		assert.Equal(t, ErrProductForbidden, err)
	})
}

func TestListShopProducts(t *testing.T) {
	mockRepo := new(MockProductRepository)
	svc := NewProductService(mockRepo)

	t.Run("성공 - 공개 상품 목록 조회", func(t *testing.T) {
		products := []*domain.Product{
			{ID: 1, Name: "상품1", Status: domain.ProductStatusPublished, Visibility: "public"},
			{ID: 2, Name: "상품2", Status: domain.ProductStatusPublished, Visibility: "public"},
		}

		req := &domain.ProductListRequest{Page: 1, Limit: 20}
		mockRepo.On("ListPublished", req).Return(products, int64(2), nil).Once()

		result, meta, err := svc.ListShopProducts(req)

		assert.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Equal(t, int64(2), meta.Total)
	})
}

func TestGetShopProduct(t *testing.T) {
	mockRepo := new(MockProductRepository)
	svc := NewProductService(mockRepo)

	t.Run("성공 - 공개 상품 조회", func(t *testing.T) {
		product := &domain.Product{
			ID:         1,
			Name:       "공개 상품",
			Status:     domain.ProductStatusPublished,
			Visibility: "public",
		}

		mockRepo.On("FindByID", uint64(1)).Return(product, nil).Once()
		mockRepo.On("IncrementViewCount", uint64(1)).Return(nil).Once()

		result, err := svc.GetShopProduct(1)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "공개 상품", result.Name)
	})

	t.Run("실패 - 비공개 상품 조회", func(t *testing.T) {
		product := &domain.Product{
			ID:         1,
			Name:       "비공개 상품",
			Status:     domain.ProductStatusDraft, // 비공개
			Visibility: "public",
		}

		mockRepo.On("FindByID", uint64(1)).Return(product, nil).Once()

		result, err := svc.GetShopProduct(1)

		assert.Error(t, err)
		assert.Equal(t, ErrProductNotFound, err)
		assert.Nil(t, result)
	})
}
