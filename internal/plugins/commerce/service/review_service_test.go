package service

import (
	"errors"
	"testing"
	"time"

	"github.com/damoang/angple-backend/internal/plugins/commerce/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"gorm.io/gorm"
)

// Mock ReviewRepository
type MockReviewRepository struct {
	mock.Mock
}

func (m *MockReviewRepository) Create(review *domain.Review) error {
	args := m.Called(review)
	return args.Error(0)
}

func (m *MockReviewRepository) Update(id uint64, review *domain.Review) error {
	args := m.Called(id, review)
	return args.Error(0)
}

func (m *MockReviewRepository) Delete(id uint64) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *MockReviewRepository) UpdateStatus(id uint64, status domain.ReviewStatus) error {
	args := m.Called(id, status)
	return args.Error(0)
}

func (m *MockReviewRepository) UpdateSellerReply(id uint64, reply string) error {
	args := m.Called(id, reply)
	return args.Error(0)
}

func (m *MockReviewRepository) FindByID(id uint64) (*domain.Review, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Review), args.Error(1)
}

func (m *MockReviewRepository) FindByUserAndOrderItem(userID, orderItemID uint64) (*domain.Review, error) {
	args := m.Called(userID, orderItemID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Review), args.Error(1)
}

func (m *MockReviewRepository) ListByProduct(productID uint64, req *domain.ReviewListRequest) ([]*domain.Review, int64, error) {
	args := m.Called(productID, req)
	return args.Get(0).([]*domain.Review), args.Get(1).(int64), args.Error(2)
}

func (m *MockReviewRepository) ListByUser(userID uint64, req *domain.ReviewListRequest) ([]*domain.Review, int64, error) {
	args := m.Called(userID, req)
	return args.Get(0).([]*domain.Review), args.Get(1).(int64), args.Error(2)
}

func (m *MockReviewRepository) ListBySeller(sellerID uint64, req *domain.ReviewListRequest) ([]*domain.Review, int64, error) {
	args := m.Called(sellerID, req)
	return args.Get(0).([]*domain.Review), args.Get(1).(int64), args.Error(2)
}

func (m *MockReviewRepository) GetProductSummary(productID uint64) (*domain.ReviewSummary, error) {
	args := m.Called(productID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.ReviewSummary), args.Error(1)
}

func (m *MockReviewRepository) UpdateProductRating(productID uint64) error {
	args := m.Called(productID)
	return args.Error(0)
}

func (m *MockReviewRepository) IncrementHelpfulCount(id uint64) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *MockReviewRepository) DecrementHelpfulCount(id uint64) error {
	args := m.Called(id)
	return args.Error(0)
}

// Mock ReviewHelpfulRepository
type MockReviewHelpfulRepository struct {
	mock.Mock
}

func (m *MockReviewHelpfulRepository) Create(helpful *domain.ReviewHelpful) error {
	args := m.Called(helpful)
	return args.Error(0)
}

func (m *MockReviewHelpfulRepository) Delete(reviewID, userID uint64) error {
	args := m.Called(reviewID, userID)
	return args.Error(0)
}

func (m *MockReviewHelpfulRepository) FindByReviewAndUser(reviewID, userID uint64) (*domain.ReviewHelpful, error) {
	args := m.Called(reviewID, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.ReviewHelpful), args.Error(1)
}

func (m *MockReviewHelpfulRepository) CountByReview(reviewID uint64) (int64, error) {
	args := m.Called(reviewID)
	return args.Get(0).(int64), args.Error(1)
}

func TestCreateReview(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name          string
		userID        uint64
		req           *domain.CreateReviewRequest
		setupMocks    func(*MockReviewRepository, *MockReviewHelpfulRepository, *MockOrderRepository, *MockProductRepository)
		expectedError error
	}{
		{
			name:   "성공 - 리뷰 작성",
			userID: 1,
			req: &domain.CreateReviewRequest{
				ProductID:   1,
				OrderItemID: 1,
				Rating:      5,
				Title:       "좋아요",
				Content:     "정말 좋은 상품입니다. 추천합니다!",
				Images:      []string{"image1.jpg", "image2.jpg"},
			},
			setupMocks: func(reviewRepo *MockReviewRepository, helpfulRepo *MockReviewHelpfulRepository, orderRepo *MockOrderRepository, productRepo *MockProductRepository) {
				orderRepo.On("FindItemByID", uint64(1)).Return(&domain.OrderItem{
					ID:        1,
					OrderID:   1,
					ProductID: 1,
				}, nil)
				orderRepo.On("FindByID", uint64(1)).Return(&domain.Order{
					ID:     1,
					UserID: 1,
					Status: domain.OrderStatusCompleted,
				}, nil)
				reviewRepo.On("FindByUserAndOrderItem", uint64(1), uint64(1)).Return(nil, gorm.ErrRecordNotFound)
				reviewRepo.On("Create", mock.AnythingOfType("*domain.Review")).Return(nil)
				reviewRepo.On("UpdateProductRating", uint64(1)).Return(nil)
			},
			expectedError: nil,
		},
		{
			name:   "실패 - 유효하지 않은 평점",
			userID: 1,
			req: &domain.CreateReviewRequest{
				ProductID:   1,
				OrderItemID: 1,
				Rating:      0,
				Content:     "리뷰 내용입니다.",
			},
			setupMocks: func(reviewRepo *MockReviewRepository, helpfulRepo *MockReviewHelpfulRepository, orderRepo *MockOrderRepository, productRepo *MockProductRepository) {
			},
			expectedError: ErrInvalidRating,
		},
		{
			name:   "실패 - 주문 아이템 없음",
			userID: 1,
			req: &domain.CreateReviewRequest{
				ProductID:   1,
				OrderItemID: 999,
				Rating:      5,
				Content:     "리뷰 내용입니다.",
			},
			setupMocks: func(reviewRepo *MockReviewRepository, helpfulRepo *MockReviewHelpfulRepository, orderRepo *MockOrderRepository, productRepo *MockProductRepository) {
				orderRepo.On("FindItemByID", uint64(999)).Return(nil, gorm.ErrRecordNotFound)
			},
			expectedError: ErrOrderItemNotFound,
		},
		{
			name:   "실패 - 다른 사용자의 주문",
			userID: 1,
			req: &domain.CreateReviewRequest{
				ProductID:   1,
				OrderItemID: 1,
				Rating:      5,
				Content:     "리뷰 내용입니다.",
			},
			setupMocks: func(reviewRepo *MockReviewRepository, helpfulRepo *MockReviewHelpfulRepository, orderRepo *MockOrderRepository, productRepo *MockProductRepository) {
				orderRepo.On("FindItemByID", uint64(1)).Return(&domain.OrderItem{
					ID:        1,
					OrderID:   1,
					ProductID: 1,
				}, nil)
				orderRepo.On("FindByID", uint64(1)).Return(&domain.Order{
					ID:     1,
					UserID: 999, // 다른 사용자
					Status: domain.OrderStatusCompleted,
				}, nil)
			},
			expectedError: ErrReviewForbidden,
		},
		{
			name:   "실패 - 미완료 주문",
			userID: 1,
			req: &domain.CreateReviewRequest{
				ProductID:   1,
				OrderItemID: 1,
				Rating:      5,
				Content:     "리뷰 내용입니다.",
			},
			setupMocks: func(reviewRepo *MockReviewRepository, helpfulRepo *MockReviewHelpfulRepository, orderRepo *MockOrderRepository, productRepo *MockProductRepository) {
				orderRepo.On("FindItemByID", uint64(1)).Return(&domain.OrderItem{
					ID:        1,
					OrderID:   1,
					ProductID: 1,
				}, nil)
				orderRepo.On("FindByID", uint64(1)).Return(&domain.Order{
					ID:     1,
					UserID: 1,
					Status: domain.OrderStatusPending, // 미완료
				}, nil)
			},
			expectedError: ErrOrderItemNotComplete,
		},
		{
			name:   "실패 - 중복 리뷰",
			userID: 1,
			req: &domain.CreateReviewRequest{
				ProductID:   1,
				OrderItemID: 1,
				Rating:      5,
				Content:     "리뷰 내용입니다.",
			},
			setupMocks: func(reviewRepo *MockReviewRepository, helpfulRepo *MockReviewHelpfulRepository, orderRepo *MockOrderRepository, productRepo *MockProductRepository) {
				orderRepo.On("FindItemByID", uint64(1)).Return(&domain.OrderItem{
					ID:        1,
					OrderID:   1,
					ProductID: 1,
				}, nil)
				orderRepo.On("FindByID", uint64(1)).Return(&domain.Order{
					ID:     1,
					UserID: 1,
					Status: domain.OrderStatusCompleted,
				}, nil)
				reviewRepo.On("FindByUserAndOrderItem", uint64(1), uint64(1)).Return(&domain.Review{
					ID:          1,
					ProductID:   1,
					UserID:      1,
					OrderItemID: 1,
					Rating:      5,
					Content:     "기존 리뷰",
					CreatedAt:   now,
				}, nil)
			},
			expectedError: ErrReviewAlreadyExists,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reviewRepo := new(MockReviewRepository)
			helpfulRepo := new(MockReviewHelpfulRepository)
			orderRepo := new(MockOrderRepository)
			productRepo := new(MockProductRepository)

			tt.setupMocks(reviewRepo, helpfulRepo, orderRepo, productRepo)

			svc := NewReviewService(reviewRepo, helpfulRepo, orderRepo, productRepo)
			result, err := svc.CreateReview(tt.userID, tt.req)

			if tt.expectedError != nil {
				assert.ErrorIs(t, err, tt.expectedError)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.req.Rating, result.Rating)
				assert.Equal(t, tt.req.Title, result.Title)
				assert.Equal(t, tt.req.Content, result.Content)
			}

			reviewRepo.AssertExpectations(t)
			orderRepo.AssertExpectations(t)
		})
	}
}

func TestUpdateReview(t *testing.T) {
	now := time.Now()
	rating := uint8(4)
	title := "수정된 제목"
	content := "수정된 내용입니다. 매우 만족합니다."

	tests := []struct {
		name          string
		userID        uint64
		reviewID      uint64
		req           *domain.UpdateReviewRequest
		setupMocks    func(*MockReviewRepository, *MockReviewHelpfulRepository, *MockOrderRepository, *MockProductRepository)
		expectedError error
	}{
		{
			name:     "성공 - 리뷰 수정",
			userID:   1,
			reviewID: 1,
			req: &domain.UpdateReviewRequest{
				Rating:  &rating,
				Title:   &title,
				Content: &content,
			},
			setupMocks: func(reviewRepo *MockReviewRepository, helpfulRepo *MockReviewHelpfulRepository, orderRepo *MockOrderRepository, productRepo *MockProductRepository) {
				reviewRepo.On("FindByID", uint64(1)).Return(&domain.Review{
					ID:        1,
					ProductID: 1,
					UserID:    1,
					Rating:    5,
					Title:     "원래 제목",
					Content:   "원래 내용입니다.",
					CreatedAt: now,
					UpdatedAt: now,
				}, nil)
				reviewRepo.On("Update", uint64(1), mock.AnythingOfType("*domain.Review")).Return(nil)
				reviewRepo.On("UpdateProductRating", uint64(1)).Return(nil)
			},
			expectedError: nil,
		},
		{
			name:     "실패 - 리뷰 없음",
			userID:   1,
			reviewID: 999,
			req: &domain.UpdateReviewRequest{
				Rating: &rating,
			},
			setupMocks: func(reviewRepo *MockReviewRepository, helpfulRepo *MockReviewHelpfulRepository, orderRepo *MockOrderRepository, productRepo *MockProductRepository) {
				reviewRepo.On("FindByID", uint64(999)).Return(nil, gorm.ErrRecordNotFound)
			},
			expectedError: ErrReviewNotFound,
		},
		{
			name:     "실패 - 다른 사용자의 리뷰",
			userID:   1,
			reviewID: 1,
			req: &domain.UpdateReviewRequest{
				Rating: &rating,
			},
			setupMocks: func(reviewRepo *MockReviewRepository, helpfulRepo *MockReviewHelpfulRepository, orderRepo *MockOrderRepository, productRepo *MockProductRepository) {
				reviewRepo.On("FindByID", uint64(1)).Return(&domain.Review{
					ID:        1,
					ProductID: 1,
					UserID:    999, // 다른 사용자
					Rating:    5,
					CreatedAt: now,
				}, nil)
			},
			expectedError: ErrReviewForbidden,
		},
		{
			name:     "실패 - 유효하지 않은 평점",
			userID:   1,
			reviewID: 1,
			req: &domain.UpdateReviewRequest{
				Rating: func() *uint8 { v := uint8(6); return &v }(),
			},
			setupMocks: func(reviewRepo *MockReviewRepository, helpfulRepo *MockReviewHelpfulRepository, orderRepo *MockOrderRepository, productRepo *MockProductRepository) {
				reviewRepo.On("FindByID", uint64(1)).Return(&domain.Review{
					ID:        1,
					ProductID: 1,
					UserID:    1,
					Rating:    5,
					CreatedAt: now,
				}, nil)
			},
			expectedError: ErrInvalidRating,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reviewRepo := new(MockReviewRepository)
			helpfulRepo := new(MockReviewHelpfulRepository)
			orderRepo := new(MockOrderRepository)
			productRepo := new(MockProductRepository)

			tt.setupMocks(reviewRepo, helpfulRepo, orderRepo, productRepo)

			svc := NewReviewService(reviewRepo, helpfulRepo, orderRepo, productRepo)
			result, err := svc.UpdateReview(tt.userID, tt.reviewID, tt.req)

			if tt.expectedError != nil {
				assert.ErrorIs(t, err, tt.expectedError)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}

			reviewRepo.AssertExpectations(t)
		})
	}
}

func TestDeleteReview(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name          string
		userID        uint64
		reviewID      uint64
		setupMocks    func(*MockReviewRepository, *MockReviewHelpfulRepository, *MockOrderRepository, *MockProductRepository)
		expectedError error
	}{
		{
			name:     "성공 - 리뷰 삭제",
			userID:   1,
			reviewID: 1,
			setupMocks: func(reviewRepo *MockReviewRepository, helpfulRepo *MockReviewHelpfulRepository, orderRepo *MockOrderRepository, productRepo *MockProductRepository) {
				reviewRepo.On("FindByID", uint64(1)).Return(&domain.Review{
					ID:        1,
					ProductID: 1,
					UserID:    1,
					Rating:    5,
					CreatedAt: now,
				}, nil)
				reviewRepo.On("Delete", uint64(1)).Return(nil)
				reviewRepo.On("UpdateProductRating", uint64(1)).Return(nil)
			},
			expectedError: nil,
		},
		{
			name:     "실패 - 리뷰 없음",
			userID:   1,
			reviewID: 999,
			setupMocks: func(reviewRepo *MockReviewRepository, helpfulRepo *MockReviewHelpfulRepository, orderRepo *MockOrderRepository, productRepo *MockProductRepository) {
				reviewRepo.On("FindByID", uint64(999)).Return(nil, gorm.ErrRecordNotFound)
			},
			expectedError: ErrReviewNotFound,
		},
		{
			name:     "실패 - 다른 사용자의 리뷰",
			userID:   1,
			reviewID: 1,
			setupMocks: func(reviewRepo *MockReviewRepository, helpfulRepo *MockReviewHelpfulRepository, orderRepo *MockOrderRepository, productRepo *MockProductRepository) {
				reviewRepo.On("FindByID", uint64(1)).Return(&domain.Review{
					ID:        1,
					ProductID: 1,
					UserID:    999, // 다른 사용자
					Rating:    5,
					CreatedAt: now,
				}, nil)
			},
			expectedError: ErrReviewForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reviewRepo := new(MockReviewRepository)
			helpfulRepo := new(MockReviewHelpfulRepository)
			orderRepo := new(MockOrderRepository)
			productRepo := new(MockProductRepository)

			tt.setupMocks(reviewRepo, helpfulRepo, orderRepo, productRepo)

			svc := NewReviewService(reviewRepo, helpfulRepo, orderRepo, productRepo)
			err := svc.DeleteReview(tt.userID, tt.reviewID)

			if tt.expectedError != nil {
				assert.ErrorIs(t, err, tt.expectedError)
			} else {
				assert.NoError(t, err)
			}

			reviewRepo.AssertExpectations(t)
		})
	}
}

func TestReplyToReview(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name          string
		sellerID      uint64
		reviewID      uint64
		req           *domain.ReplyReviewRequest
		setupMocks    func(*MockReviewRepository, *MockReviewHelpfulRepository, *MockOrderRepository, *MockProductRepository)
		expectedError error
	}{
		{
			name:     "성공 - 판매자 답글",
			sellerID: 1,
			reviewID: 1,
			req: &domain.ReplyReviewRequest{
				Reply: "리뷰 감사합니다. 앞으로도 좋은 상품으로 보답하겠습니다.",
			},
			setupMocks: func(reviewRepo *MockReviewRepository, helpfulRepo *MockReviewHelpfulRepository, orderRepo *MockOrderRepository, productRepo *MockProductRepository) {
				reviewRepo.On("FindByID", uint64(1)).Return(&domain.Review{
					ID:        1,
					ProductID: 1,
					UserID:    2,
					Rating:    5,
					Content:   "좋은 상품입니다!",
					CreatedAt: now,
				}, nil).Once()
				productRepo.On("FindByID", uint64(1)).Return(&domain.Product{
					ID:       1,
					SellerID: 1, // 판매자 일치
					Name:     "테스트 상품",
				}, nil)
				reviewRepo.On("UpdateSellerReply", uint64(1), "리뷰 감사합니다. 앞으로도 좋은 상품으로 보답하겠습니다.").Return(nil)
				reviewRepo.On("FindByID", uint64(1)).Return(&domain.Review{
					ID:          1,
					ProductID:   1,
					UserID:      2,
					Rating:      5,
					Content:     "좋은 상품입니다!",
					SellerReply: "리뷰 감사합니다. 앞으로도 좋은 상품으로 보답하겠습니다.",
					RepliedAt:   &now,
					CreatedAt:   now,
				}, nil).Once()
			},
			expectedError: nil,
		},
		{
			name:     "실패 - 리뷰 없음",
			sellerID: 1,
			reviewID: 999,
			req: &domain.ReplyReviewRequest{
				Reply: "감사합니다.",
			},
			setupMocks: func(reviewRepo *MockReviewRepository, helpfulRepo *MockReviewHelpfulRepository, orderRepo *MockOrderRepository, productRepo *MockProductRepository) {
				reviewRepo.On("FindByID", uint64(999)).Return(nil, gorm.ErrRecordNotFound)
			},
			expectedError: ErrReviewNotFound,
		},
		{
			name:     "실패 - 다른 판매자의 상품",
			sellerID: 1,
			reviewID: 1,
			req: &domain.ReplyReviewRequest{
				Reply: "감사합니다.",
			},
			setupMocks: func(reviewRepo *MockReviewRepository, helpfulRepo *MockReviewHelpfulRepository, orderRepo *MockOrderRepository, productRepo *MockProductRepository) {
				reviewRepo.On("FindByID", uint64(1)).Return(&domain.Review{
					ID:        1,
					ProductID: 1,
					UserID:    2,
					Rating:    5,
					CreatedAt: now,
				}, nil)
				productRepo.On("FindByID", uint64(1)).Return(&domain.Product{
					ID:       1,
					SellerID: 999, // 다른 판매자
					Name:     "테스트 상품",
				}, nil)
			},
			expectedError: ErrSellerReplyForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reviewRepo := new(MockReviewRepository)
			helpfulRepo := new(MockReviewHelpfulRepository)
			orderRepo := new(MockOrderRepository)
			productRepo := new(MockProductRepository)

			tt.setupMocks(reviewRepo, helpfulRepo, orderRepo, productRepo)

			svc := NewReviewService(reviewRepo, helpfulRepo, orderRepo, productRepo)
			result, err := svc.ReplyToReview(tt.sellerID, tt.reviewID, tt.req)

			if tt.expectedError != nil {
				assert.ErrorIs(t, err, tt.expectedError)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.req.Reply, result.SellerReply)
			}

			reviewRepo.AssertExpectations(t)
			productRepo.AssertExpectations(t)
		})
	}
}

func TestToggleHelpful(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name          string
		userID        uint64
		reviewID      uint64
		setupMocks    func(*MockReviewRepository, *MockReviewHelpfulRepository, *MockOrderRepository, *MockProductRepository)
		expectedValue bool
		expectedError error
	}{
		{
			name:     "성공 - 도움됨 추가",
			userID:   1,
			reviewID: 1,
			setupMocks: func(reviewRepo *MockReviewRepository, helpfulRepo *MockReviewHelpfulRepository, orderRepo *MockOrderRepository, productRepo *MockProductRepository) {
				reviewRepo.On("FindByID", uint64(1)).Return(&domain.Review{
					ID:        1,
					ProductID: 1,
					UserID:    2,
					Rating:    5,
					CreatedAt: now,
				}, nil)
				helpfulRepo.On("FindByReviewAndUser", uint64(1), uint64(1)).Return(nil, gorm.ErrRecordNotFound)
				helpfulRepo.On("Create", mock.AnythingOfType("*domain.ReviewHelpful")).Return(nil)
				reviewRepo.On("IncrementHelpfulCount", uint64(1)).Return(nil)
			},
			expectedValue: true,
			expectedError: nil,
		},
		{
			name:     "성공 - 도움됨 취소",
			userID:   1,
			reviewID: 1,
			setupMocks: func(reviewRepo *MockReviewRepository, helpfulRepo *MockReviewHelpfulRepository, orderRepo *MockOrderRepository, productRepo *MockProductRepository) {
				reviewRepo.On("FindByID", uint64(1)).Return(&domain.Review{
					ID:        1,
					ProductID: 1,
					UserID:    2,
					Rating:    5,
					CreatedAt: now,
				}, nil)
				helpfulRepo.On("FindByReviewAndUser", uint64(1), uint64(1)).Return(&domain.ReviewHelpful{
					ID:        1,
					ReviewID:  1,
					UserID:    1,
					CreatedAt: now,
				}, nil)
				helpfulRepo.On("Delete", uint64(1), uint64(1)).Return(nil)
				reviewRepo.On("DecrementHelpfulCount", uint64(1)).Return(nil)
			},
			expectedValue: false,
			expectedError: nil,
		},
		{
			name:     "실패 - 리뷰 없음",
			userID:   1,
			reviewID: 999,
			setupMocks: func(reviewRepo *MockReviewRepository, helpfulRepo *MockReviewHelpfulRepository, orderRepo *MockOrderRepository, productRepo *MockProductRepository) {
				reviewRepo.On("FindByID", uint64(999)).Return(nil, gorm.ErrRecordNotFound)
			},
			expectedValue: false,
			expectedError: ErrReviewNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reviewRepo := new(MockReviewRepository)
			helpfulRepo := new(MockReviewHelpfulRepository)
			orderRepo := new(MockOrderRepository)
			productRepo := new(MockProductRepository)

			tt.setupMocks(reviewRepo, helpfulRepo, orderRepo, productRepo)

			svc := NewReviewService(reviewRepo, helpfulRepo, orderRepo, productRepo)
			result, err := svc.ToggleHelpful(tt.userID, tt.reviewID)

			if tt.expectedError != nil {
				assert.ErrorIs(t, err, tt.expectedError)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedValue, result)
			}

			reviewRepo.AssertExpectations(t)
			helpfulRepo.AssertExpectations(t)
		})
	}
}

func TestListProductReviews(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name          string
		productID     uint64
		req           *domain.ReviewListRequest
		setupMocks    func(*MockReviewRepository, *MockReviewHelpfulRepository, *MockOrderRepository, *MockProductRepository)
		expectedCount int
		expectedTotal int64
		expectedError error
	}{
		{
			name:      "성공 - 상품 리뷰 목록 조회",
			productID: 1,
			req: &domain.ReviewListRequest{
				Page:  1,
				Limit: 10,
			},
			setupMocks: func(reviewRepo *MockReviewRepository, helpfulRepo *MockReviewHelpfulRepository, orderRepo *MockOrderRepository, productRepo *MockProductRepository) {
				reviewRepo.On("ListByProduct", uint64(1), mock.AnythingOfType("*domain.ReviewListRequest")).Return([]*domain.Review{
					{ID: 1, ProductID: 1, UserID: 1, Rating: 5, Content: "좋아요", CreatedAt: now},
					{ID: 2, ProductID: 1, UserID: 2, Rating: 4, Content: "괜찮아요", CreatedAt: now},
				}, int64(2), nil)
			},
			expectedCount: 2,
			expectedTotal: 2,
			expectedError: nil,
		},
		{
			name:      "성공 - 빈 목록",
			productID: 1,
			req: &domain.ReviewListRequest{
				Page:  1,
				Limit: 10,
			},
			setupMocks: func(reviewRepo *MockReviewRepository, helpfulRepo *MockReviewHelpfulRepository, orderRepo *MockOrderRepository, productRepo *MockProductRepository) {
				reviewRepo.On("ListByProduct", uint64(1), mock.AnythingOfType("*domain.ReviewListRequest")).Return([]*domain.Review{}, int64(0), nil)
			},
			expectedCount: 0,
			expectedTotal: 0,
			expectedError: nil,
		},
		{
			name:      "실패 - 조회 오류",
			productID: 1,
			req: &domain.ReviewListRequest{
				Page:  1,
				Limit: 10,
			},
			setupMocks: func(reviewRepo *MockReviewRepository, helpfulRepo *MockReviewHelpfulRepository, orderRepo *MockOrderRepository, productRepo *MockProductRepository) {
				reviewRepo.On("ListByProduct", uint64(1), mock.AnythingOfType("*domain.ReviewListRequest")).Return([]*domain.Review(nil), int64(0), errors.New("database error"))
			},
			expectedCount: 0,
			expectedTotal: 0,
			expectedError: errors.New("database error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reviewRepo := new(MockReviewRepository)
			helpfulRepo := new(MockReviewHelpfulRepository)
			orderRepo := new(MockOrderRepository)
			productRepo := new(MockProductRepository)

			tt.setupMocks(reviewRepo, helpfulRepo, orderRepo, productRepo)

			svc := NewReviewService(reviewRepo, helpfulRepo, orderRepo, productRepo)
			result, total, err := svc.ListProductReviews(tt.productID, tt.req)

			if tt.expectedError != nil {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, result, tt.expectedCount)
				assert.Equal(t, tt.expectedTotal, total)
			}

			reviewRepo.AssertExpectations(t)
		})
	}
}

func TestGetProductReviewSummary(t *testing.T) {
	tests := []struct {
		name          string
		productID     uint64
		setupMocks    func(*MockReviewRepository, *MockReviewHelpfulRepository, *MockOrderRepository, *MockProductRepository)
		expectedError error
	}{
		{
			name:      "성공 - 리뷰 요약 조회",
			productID: 1,
			setupMocks: func(reviewRepo *MockReviewRepository, helpfulRepo *MockReviewHelpfulRepository, orderRepo *MockOrderRepository, productRepo *MockProductRepository) {
				reviewRepo.On("GetProductSummary", uint64(1)).Return(&domain.ReviewSummary{
					ProductID:     1,
					TotalCount:    10,
					AverageRating: 4.5,
					RatingCounts: map[uint8]int64{
						5: 5,
						4: 3,
						3: 2,
					},
				}, nil)
			},
			expectedError: nil,
		},
		{
			name:      "실패 - 조회 오류",
			productID: 1,
			setupMocks: func(reviewRepo *MockReviewRepository, helpfulRepo *MockReviewHelpfulRepository, orderRepo *MockOrderRepository, productRepo *MockProductRepository) {
				reviewRepo.On("GetProductSummary", uint64(1)).Return(nil, errors.New("database error"))
			},
			expectedError: errors.New("database error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reviewRepo := new(MockReviewRepository)
			helpfulRepo := new(MockReviewHelpfulRepository)
			orderRepo := new(MockOrderRepository)
			productRepo := new(MockProductRepository)

			tt.setupMocks(reviewRepo, helpfulRepo, orderRepo, productRepo)

			svc := NewReviewService(reviewRepo, helpfulRepo, orderRepo, productRepo)
			result, err := svc.GetProductReviewSummary(tt.productID)

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.productID, result.ProductID)
			}

			reviewRepo.AssertExpectations(t)
		})
	}
}
