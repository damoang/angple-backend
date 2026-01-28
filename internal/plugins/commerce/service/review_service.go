package service

import (
	"errors"

	"github.com/damoang/angple-backend/internal/plugins/commerce/domain"
	"github.com/damoang/angple-backend/internal/plugins/commerce/repository"
	"gorm.io/gorm"
)

// 리뷰 에러 정의
var (
	ErrReviewNotFound       = errors.New("review not found")
	ErrReviewAlreadyExists  = errors.New("review already exists for this order item")
	ErrReviewForbidden      = errors.New("you are not the owner of this review")
	ErrOrderItemNotComplete = errors.New("order item is not completed")
	ErrInvalidRating        = errors.New("rating must be between 1 and 5")
	ErrSellerReplyForbidden = errors.New("you are not the seller of this product")
)

// ReviewService 리뷰 서비스 인터페이스
type ReviewService interface {
	// 리뷰 CRUD
	CreateReview(userID uint64, req *domain.CreateReviewRequest) (*domain.ReviewResponse, error)
	UpdateReview(userID uint64, reviewID uint64, req *domain.UpdateReviewRequest) (*domain.ReviewResponse, error)
	DeleteReview(userID uint64, reviewID uint64) error
	GetReview(reviewID uint64) (*domain.ReviewResponse, error)

	// 리뷰 목록
	ListProductReviews(productID uint64, req *domain.ReviewListRequest) ([]*domain.ReviewResponse, int64, error)
	ListUserReviews(userID uint64, req *domain.ReviewListRequest) ([]*domain.ReviewResponse, int64, error)
	ListSellerReviews(sellerID uint64, req *domain.ReviewListRequest) ([]*domain.ReviewResponse, int64, error)

	// 판매자 기능
	ReplyToReview(sellerID uint64, reviewID uint64, req *domain.ReplyReviewRequest) (*domain.ReviewResponse, error)

	// 도움됨 기능
	ToggleHelpful(userID uint64, reviewID uint64) (bool, error)

	// 통계
	GetProductReviewSummary(productID uint64) (*domain.ReviewSummary, error)
}

// reviewService 구현체
type reviewService struct {
	reviewRepo        repository.ReviewRepository
	reviewHelpfulRepo repository.ReviewHelpfulRepository
	orderRepo         repository.OrderRepository
	productRepo       repository.ProductRepository
}

// NewReviewService 생성자
func NewReviewService(
	reviewRepo repository.ReviewRepository,
	reviewHelpfulRepo repository.ReviewHelpfulRepository,
	orderRepo repository.OrderRepository,
	productRepo repository.ProductRepository,
) ReviewService {
	return &reviewService{
		reviewRepo:        reviewRepo,
		reviewHelpfulRepo: reviewHelpfulRepo,
		orderRepo:         orderRepo,
		productRepo:       productRepo,
	}
}

// CreateReview 리뷰 작성
func (s *reviewService) CreateReview(userID uint64, req *domain.CreateReviewRequest) (*domain.ReviewResponse, error) {
	// 평점 검증
	if req.Rating < 1 || req.Rating > 5 {
		return nil, ErrInvalidRating
	}

	// 주문 아이템 조회 및 검증
	orderItem, err := s.orderRepo.FindItemByID(req.OrderItemID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrderItemNotFound
		}
		return nil, err
	}

	// 상품 ID 검증
	if orderItem.ProductID != req.ProductID {
		return nil, errors.New("product ID does not match order item")
	}

	// 주문 아이템의 주문 조회
	order, err := s.orderRepo.FindByID(orderItem.OrderID)
	if err != nil {
		return nil, err
	}

	// 사용자 검증
	if order.UserID != userID {
		return nil, ErrReviewForbidden
	}

	// 주문 상태 검증 (completed 또는 delivered 상태에서만 리뷰 가능)
	if order.Status != domain.OrderStatusCompleted && order.Status != domain.OrderStatusDelivered {
		return nil, ErrOrderItemNotComplete
	}

	// 중복 리뷰 검증
	existingReview, err := s.reviewRepo.FindByUserAndOrderItem(userID, req.OrderItemID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if existingReview != nil {
		return nil, ErrReviewAlreadyExists
	}

	// 리뷰 생성
	review := &domain.Review{
		ProductID:   req.ProductID,
		UserID:      userID,
		OrderItemID: req.OrderItemID,
		Rating:      req.Rating,
		Title:       req.Title,
		Content:     req.Content,
		Status:      domain.ReviewStatusApproved, // 자동 승인 (필요시 pending으로 변경)
		IsVerified:  true,
	}

	// 이미지 설정
	if len(req.Images) > 0 {
		if err := review.SetImages(req.Images); err != nil {
			return nil, err
		}
	}

	if err := s.reviewRepo.Create(review); err != nil {
		return nil, err
	}

	// 상품 평점 업데이트
	if err := s.reviewRepo.UpdateProductRating(req.ProductID); err != nil {
		// 로그만 기록하고 계속 진행 (트랜잭션 아님)
	}

	return review.ToResponse(), nil
}

// UpdateReview 리뷰 수정
func (s *reviewService) UpdateReview(userID uint64, reviewID uint64, req *domain.UpdateReviewRequest) (*domain.ReviewResponse, error) {
	review, err := s.reviewRepo.FindByID(reviewID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrReviewNotFound
		}
		return nil, err
	}

	// 소유자 검증
	if review.UserID != userID {
		return nil, ErrReviewForbidden
	}

	// 업데이트 적용
	if req.Rating != nil {
		if *req.Rating < 1 || *req.Rating > 5 {
			return nil, ErrInvalidRating
		}
		review.Rating = *req.Rating
	}
	if req.Title != nil {
		review.Title = *req.Title
	}
	if req.Content != nil {
		review.Content = *req.Content
	}
	if req.Images != nil {
		if err := review.SetImages(req.Images); err != nil {
			return nil, err
		}
	}

	if err := s.reviewRepo.Update(reviewID, review); err != nil {
		return nil, err
	}

	// 상품 평점 업데이트
	if err := s.reviewRepo.UpdateProductRating(review.ProductID); err != nil {
		// 로그만 기록
	}

	return review.ToResponse(), nil
}

// DeleteReview 리뷰 삭제
func (s *reviewService) DeleteReview(userID uint64, reviewID uint64) error {
	review, err := s.reviewRepo.FindByID(reviewID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrReviewNotFound
		}
		return err
	}

	// 소유자 검증
	if review.UserID != userID {
		return ErrReviewForbidden
	}

	productID := review.ProductID

	if err := s.reviewRepo.Delete(reviewID); err != nil {
		return err
	}

	// 상품 평점 업데이트
	if err := s.reviewRepo.UpdateProductRating(productID); err != nil {
		// 로그만 기록
	}

	return nil
}

// GetReview 리뷰 조회
func (s *reviewService) GetReview(reviewID uint64) (*domain.ReviewResponse, error) {
	review, err := s.reviewRepo.FindByID(reviewID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrReviewNotFound
		}
		return nil, err
	}
	return review.ToResponse(), nil
}

// ListProductReviews 상품 리뷰 목록 조회
func (s *reviewService) ListProductReviews(productID uint64, req *domain.ReviewListRequest) ([]*domain.ReviewResponse, int64, error) {
	reviews, total, err := s.reviewRepo.ListByProduct(productID, req)
	if err != nil {
		return nil, 0, err
	}

	responses := make([]*domain.ReviewResponse, 0, len(reviews))
	for _, review := range reviews {
		responses = append(responses, review.ToResponse())
	}

	return responses, total, nil
}

// ListUserReviews 사용자 리뷰 목록 조회
func (s *reviewService) ListUserReviews(userID uint64, req *domain.ReviewListRequest) ([]*domain.ReviewResponse, int64, error) {
	reviews, total, err := s.reviewRepo.ListByUser(userID, req)
	if err != nil {
		return nil, 0, err
	}

	responses := make([]*domain.ReviewResponse, 0, len(reviews))
	for _, review := range reviews {
		responses = append(responses, review.ToResponse())
	}

	return responses, total, nil
}

// ListSellerReviews 판매자 리뷰 목록 조회
func (s *reviewService) ListSellerReviews(sellerID uint64, req *domain.ReviewListRequest) ([]*domain.ReviewResponse, int64, error) {
	reviews, total, err := s.reviewRepo.ListBySeller(sellerID, req)
	if err != nil {
		return nil, 0, err
	}

	responses := make([]*domain.ReviewResponse, 0, len(reviews))
	for _, review := range reviews {
		responses = append(responses, review.ToResponse())
	}

	return responses, total, nil
}

// ReplyToReview 판매자 답글
func (s *reviewService) ReplyToReview(sellerID uint64, reviewID uint64, req *domain.ReplyReviewRequest) (*domain.ReviewResponse, error) {
	review, err := s.reviewRepo.FindByID(reviewID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrReviewNotFound
		}
		return nil, err
	}

	// 상품 조회하여 판매자 검증
	product, err := s.productRepo.FindByID(review.ProductID)
	if err != nil {
		return nil, err
	}
	if product.SellerID != sellerID {
		return nil, ErrSellerReplyForbidden
	}

	// 답글 업데이트
	if err := s.reviewRepo.UpdateSellerReply(reviewID, req.Reply); err != nil {
		return nil, err
	}

	// 업데이트된 리뷰 조회
	review, err = s.reviewRepo.FindByID(reviewID)
	if err != nil {
		return nil, err
	}

	return review.ToResponse(), nil
}

// ToggleHelpful 도움됨 토글
func (s *reviewService) ToggleHelpful(userID uint64, reviewID uint64) (bool, error) {
	// 리뷰 존재 확인
	_, err := s.reviewRepo.FindByID(reviewID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, ErrReviewNotFound
		}
		return false, err
	}

	// 기존 도움됨 확인
	existing, err := s.reviewHelpfulRepo.FindByReviewAndUser(reviewID, userID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, err
	}

	if existing != nil {
		// 도움됨 취소
		if err := s.reviewHelpfulRepo.Delete(reviewID, userID); err != nil {
			return false, err
		}
		if err := s.reviewRepo.DecrementHelpfulCount(reviewID); err != nil {
			return false, err
		}
		return false, nil
	}

	// 도움됨 추가
	helpful := &domain.ReviewHelpful{
		ReviewID: reviewID,
		UserID:   userID,
	}
	if err := s.reviewHelpfulRepo.Create(helpful); err != nil {
		return false, err
	}
	if err := s.reviewRepo.IncrementHelpfulCount(reviewID); err != nil {
		return true, err
	}

	return true, nil
}

// GetProductReviewSummary 상품 리뷰 요약 조회
func (s *reviewService) GetProductReviewSummary(productID uint64) (*domain.ReviewSummary, error) {
	return s.reviewRepo.GetProductSummary(productID)
}
