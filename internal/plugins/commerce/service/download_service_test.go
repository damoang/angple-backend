package service

import (
	"testing"
	"time"

	"github.com/damoang/angple-backend/internal/plugins/commerce/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"gorm.io/gorm"
)

// MockDownloadRepository 다운로드 저장소 목
type MockDownloadRepository struct {
	mock.Mock
}

func (m *MockDownloadRepository) Create(download *domain.Download) error {
	args := m.Called(download)
	return args.Error(0)
}

func (m *MockDownloadRepository) Update(id uint64, download *domain.Download) error {
	args := m.Called(id, download)
	return args.Error(0)
}

func (m *MockDownloadRepository) IncrementDownloadCount(id uint64) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *MockDownloadRepository) FindByID(id uint64) (*domain.Download, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Download), args.Error(1)
}

func (m *MockDownloadRepository) FindByToken(token string) (*domain.Download, error) {
	args := m.Called(token)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Download), args.Error(1)
}

func (m *MockDownloadRepository) FindByTokenWithFile(token string) (*domain.Download, error) {
	args := m.Called(token)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Download), args.Error(1)
}

func (m *MockDownloadRepository) FindByOrderItemAndFile(orderItemID, fileID uint64) (*domain.Download, error) {
	args := m.Called(orderItemID, fileID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Download), args.Error(1)
}

func (m *MockDownloadRepository) ListByOrderItem(orderItemID uint64) ([]*domain.Download, error) {
	args := m.Called(orderItemID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.Download), args.Error(1)
}

func (m *MockDownloadRepository) ListByUser(userID uint64) ([]*domain.Download, error) {
	args := m.Called(userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.Download), args.Error(1)
}

func (m *MockDownloadRepository) GenerateToken() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

// MockProductFileRepository 상품 파일 저장소 목
type MockProductFileRepository struct {
	mock.Mock
}

func (m *MockProductFileRepository) Create(file *domain.ProductFile) error {
	args := m.Called(file)
	return args.Error(0)
}

func (m *MockProductFileRepository) Update(id uint64, file *domain.ProductFile) error {
	args := m.Called(id, file)
	return args.Error(0)
}

func (m *MockProductFileRepository) Delete(id uint64) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *MockProductFileRepository) FindByID(id uint64) (*domain.ProductFile, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.ProductFile), args.Error(1)
}

func (m *MockProductFileRepository) ListByProduct(productID uint64) ([]*domain.ProductFile, error) {
	args := m.Called(productID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.ProductFile), args.Error(1)
}

// MockOrderRepositoryForDownload 주문 저장소 목 (다운로드용)
type MockOrderRepositoryForDownload struct {
	mock.Mock
}

func (m *MockOrderRepositoryForDownload) Create(order *domain.Order) error {
	args := m.Called(order)
	return args.Error(0)
}

func (m *MockOrderRepositoryForDownload) CreateWithItems(order *domain.Order, items []domain.OrderItem) error {
	args := m.Called(order, items)
	return args.Error(0)
}

func (m *MockOrderRepositoryForDownload) Update(id uint64, order *domain.Order) error {
	args := m.Called(id, order)
	return args.Error(0)
}

func (m *MockOrderRepositoryForDownload) UpdateStatus(id uint64, status domain.OrderStatus) error {
	args := m.Called(id, status)
	return args.Error(0)
}

func (m *MockOrderRepositoryForDownload) FindByID(id uint64) (*domain.Order, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Order), args.Error(1)
}

func (m *MockOrderRepositoryForDownload) FindByIDWithItems(id uint64) (*domain.Order, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Order), args.Error(1)
}

func (m *MockOrderRepositoryForDownload) FindByOrderNumber(orderNumber string) (*domain.Order, error) {
	args := m.Called(orderNumber)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Order), args.Error(1)
}

func (m *MockOrderRepositoryForDownload) FindByOrderNumberWithItems(orderNumber string) (*domain.Order, error) {
	args := m.Called(orderNumber)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Order), args.Error(1)
}

func (m *MockOrderRepositoryForDownload) ListByUser(userID uint64, req *domain.OrderListRequest) ([]*domain.Order, int64, error) {
	args := m.Called(userID, req)
	return args.Get(0).([]*domain.Order), args.Get(1).(int64), args.Error(2)
}

func (m *MockOrderRepositoryForDownload) ListByUserWithItems(userID uint64, req *domain.OrderListRequest) ([]*domain.Order, int64, error) {
	args := m.Called(userID, req)
	return args.Get(0).([]*domain.Order), args.Get(1).(int64), args.Error(2)
}

func (m *MockOrderRepositoryForDownload) ListBySeller(sellerID uint64, req *domain.OrderListRequest) ([]*domain.Order, int64, error) {
	args := m.Called(sellerID, req)
	return args.Get(0).([]*domain.Order), args.Get(1).(int64), args.Error(2)
}

func (m *MockOrderRepositoryForDownload) FindItemByID(itemID uint64) (*domain.OrderItem, error) {
	args := m.Called(itemID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.OrderItem), args.Error(1)
}

func (m *MockOrderRepositoryForDownload) UpdateItemStatus(itemID uint64, status domain.OrderItemStatus) error {
	args := m.Called(itemID, status)
	return args.Error(0)
}

func (m *MockOrderRepositoryForDownload) GenerateOrderNumber() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func TestListUserDownloads(t *testing.T) {
	t.Run("성공 - 사용자 다운로드 목록 조회", func(t *testing.T) {
		downloadRepo := new(MockDownloadRepository)
		orderRepo := new(MockOrderRepositoryForDownload)
		productFileRepo := new(MockProductFileRepository)

		svc := NewDownloadService(downloadRepo, orderRepo, productFileRepo)

		downloads := []*domain.Download{
			{
				ID:            1,
				OrderItemID:   1,
				FileID:        1,
				UserID:        1,
				DownloadToken: "token1",
				DownloadCount: 0,
			},
			{
				ID:            2,
				OrderItemID:   2,
				FileID:        2,
				UserID:        1,
				DownloadToken: "token2",
				DownloadCount: 1,
			},
		}

		downloadRepo.On("ListByUser", uint64(1)).Return(downloads, nil)

		result, err := svc.ListUserDownloads(1)

		assert.NoError(t, err)
		assert.Len(t, result, 2)
	})
}

func TestGenerateDownloadURL(t *testing.T) {
	t.Run("성공 - 다운로드 URL 생성", func(t *testing.T) {
		downloadRepo := new(MockDownloadRepository)
		orderRepo := new(MockOrderRepositoryForDownload)
		productFileRepo := new(MockProductFileRepository)

		svc := NewDownloadService(downloadRepo, orderRepo, productFileRepo)

		expiresAt := time.Now().Add(24 * time.Hour)
		download := &domain.Download{
			ID:            1,
			OrderItemID:   1,
			FileID:        1,
			UserID:        1,
			DownloadToken: "test-token",
			DownloadCount: 0,
			ExpiresAt:     &expiresAt,
		}

		productFile := &domain.ProductFile{
			ID:        1,
			ProductID: 1,
			FileName:  "test.pdf",
			FilePath:  "/files/test.pdf",
			FileSize:  1024,
		}

		downloadRepo.On("FindByOrderItemAndFile", uint64(1), uint64(1)).Return(download, nil)
		productFileRepo.On("FindByID", uint64(1)).Return(productFile, nil)

		result, err := svc.GenerateDownloadURL(1, 1, 1, "http://localhost:8082", "test-secret")

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Contains(t, result.DownloadURL, "http://localhost:8082")
		assert.Equal(t, "test.pdf", result.FileName)
	})

	t.Run("실패 - 다운로드 한도 초과", func(t *testing.T) {
		downloadRepo := new(MockDownloadRepository)
		orderRepo := new(MockOrderRepositoryForDownload)
		productFileRepo := new(MockProductFileRepository)

		svc := NewDownloadService(downloadRepo, orderRepo, productFileRepo)

		expiresAt := time.Now().Add(24 * time.Hour)
		download := &domain.Download{
			ID:            1,
			OrderItemID:   1,
			FileID:        1,
			UserID:        1,
			DownloadToken: "test-token",
			DownloadCount: 5, // 최대 횟수 도달 (기본 5회 제한)
			ExpiresAt:     &expiresAt,
		}

		downloadRepo.On("FindByOrderItemAndFile", uint64(1), uint64(1)).Return(download, nil)

		result, err := svc.GenerateDownloadURL(1, 1, 1, "http://localhost:8082", "test-secret")

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, ErrDownloadLimitReached, err)
	})

	t.Run("실패 - 다운로드 기한 만료", func(t *testing.T) {
		downloadRepo := new(MockDownloadRepository)
		orderRepo := new(MockOrderRepositoryForDownload)
		productFileRepo := new(MockProductFileRepository)

		svc := NewDownloadService(downloadRepo, orderRepo, productFileRepo)

		expiresAt := time.Now().Add(-24 * time.Hour) // 만료됨
		download := &domain.Download{
			ID:            1,
			OrderItemID:   1,
			FileID:        1,
			UserID:        1,
			DownloadToken: "test-token",
			DownloadCount: 0,
			ExpiresAt:     &expiresAt,
		}

		downloadRepo.On("FindByOrderItemAndFile", uint64(1), uint64(1)).Return(download, nil)

		result, err := svc.GenerateDownloadURL(1, 1, 1, "http://localhost:8082", "test-secret")

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, ErrDownloadExpired, err)
	})

	t.Run("실패 - 다른 사용자의 다운로드", func(t *testing.T) {
		downloadRepo := new(MockDownloadRepository)
		orderRepo := new(MockOrderRepositoryForDownload)
		productFileRepo := new(MockProductFileRepository)

		svc := NewDownloadService(downloadRepo, orderRepo, productFileRepo)

		expiresAt := time.Now().Add(24 * time.Hour)
		download := &domain.Download{
			ID:            1,
			OrderItemID:   1,
			FileID:        1,
			UserID:        2, // 다른 사용자
			DownloadToken: "test-token",
			DownloadCount: 0,
			ExpiresAt:     &expiresAt,
		}

		downloadRepo.On("FindByOrderItemAndFile", uint64(1), uint64(1)).Return(download, nil)

		result, err := svc.GenerateDownloadURL(1, 1, 1, "http://localhost:8082", "test-secret") // 사용자 ID: 1

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, ErrDownloadForbidden, err)
	})

	t.Run("실패 - 다운로드 없음", func(t *testing.T) {
		downloadRepo := new(MockDownloadRepository)
		orderRepo := new(MockOrderRepositoryForDownload)
		productFileRepo := new(MockProductFileRepository)

		svc := NewDownloadService(downloadRepo, orderRepo, productFileRepo)

		downloadRepo.On("FindByOrderItemAndFile", uint64(1), uint64(999)).Return(nil, gorm.ErrRecordNotFound)

		result, err := svc.GenerateDownloadURL(1, 1, 999, "http://localhost:8082", "test-secret")

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, ErrDownloadNotFound, err)
	})
}

func TestVerifySignature(t *testing.T) {
	downloadRepo := new(MockDownloadRepository)
	orderRepo := new(MockOrderRepositoryForDownload)
	productFileRepo := new(MockProductFileRepository)

	svc := NewDownloadService(downloadRepo, orderRepo, productFileRepo)

	t.Run("실패 - 빈 서명", func(t *testing.T) {
		// 만료되지 않은 시간으로 설정
		expiresAt := time.Now().Add(1 * time.Hour)
		token := "test-token"
		secretKey := "test-secret"

		// 빈 서명은 항상 실패해야 함
		result := svc.VerifySignature(token, "", secretKey, expiresAt)

		assert.False(t, result)
	})

	t.Run("실패 - 만료된 토큰", func(t *testing.T) {
		expiresAt := time.Now().Add(-1 * time.Hour) // 이미 만료
		token := "test-token"
		secretKey := "test-secret"

		result := svc.VerifySignature(token, "any-signature", secretKey, expiresAt)

		assert.False(t, result)
	})
}

func TestCreateDownloadAccess(t *testing.T) {
	t.Run("성공 - 다운로드 권한 생성", func(t *testing.T) {
		downloadRepo := new(MockDownloadRepository)
		orderRepo := new(MockOrderRepositoryForDownload)
		productFileRepo := new(MockProductFileRepository)

		svc := NewDownloadService(downloadRepo, orderRepo, productFileRepo)

		orderItem := &domain.OrderItem{
			ID:          1,
			OrderID:     1,
			ProductID:   1,
			ProductType: domain.ProductTypeDigital,
		}

		productFiles := []*domain.ProductFile{
			{
				ID:        1,
				ProductID: 1,
				FileName:  "file1.pdf",
				FilePath:  "/files/file1.pdf",
			},
		}

		orderRepo.On("FindItemByID", uint64(1)).Return(orderItem, nil)
		productFileRepo.On("ListByProduct", uint64(1)).Return(productFiles, nil)
		downloadRepo.On("FindByOrderItemAndFile", uint64(1), uint64(1)).Return(nil, gorm.ErrRecordNotFound)
		downloadRepo.On("Create", mock.AnythingOfType("*domain.Download")).Return(nil)

		result, err := svc.CreateDownloadAccess(1, 1)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Len(t, result, 1)
	})

	t.Run("실패 - 주문 아이템 없음", func(t *testing.T) {
		downloadRepo := new(MockDownloadRepository)
		orderRepo := new(MockOrderRepositoryForDownload)
		productFileRepo := new(MockProductFileRepository)

		svc := NewDownloadService(downloadRepo, orderRepo, productFileRepo)

		orderRepo.On("FindItemByID", uint64(999)).Return(nil, gorm.ErrRecordNotFound)

		result, err := svc.CreateDownloadAccess(999, 1)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, ErrOrderItemNotFound, err)
	})

	t.Run("실패 - 디지털 상품이 아님", func(t *testing.T) {
		downloadRepo := new(MockDownloadRepository)
		orderRepo := new(MockOrderRepositoryForDownload)
		productFileRepo := new(MockProductFileRepository)

		svc := NewDownloadService(downloadRepo, orderRepo, productFileRepo)

		orderItem := &domain.OrderItem{
			ID:          1,
			OrderID:     1,
			ProductID:   1,
			ProductType: domain.ProductTypePhysical, // 실물 상품
		}

		orderRepo.On("FindItemByID", uint64(1)).Return(orderItem, nil)

		result, err := svc.CreateDownloadAccess(1, 1)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, ErrNotDigitalProduct, err)
	})
}
