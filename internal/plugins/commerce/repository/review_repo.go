package repository

import (
	"time"

	"github.com/damoang/angple-backend/internal/plugins/commerce/domain"
	"gorm.io/gorm"
)

// ReviewRepository 리뷰 저장소 인터페이스
type ReviewRepository interface {
	// 생성/수정/삭제
	Create(review *domain.Review) error
	Update(id uint64, review *domain.Review) error
	Delete(id uint64) error
	UpdateStatus(id uint64, status domain.ReviewStatus) error
	UpdateSellerReply(id uint64, reply string) error

	// 조회
	FindByID(id uint64) (*domain.Review, error)
	FindByUserAndOrderItem(userID, orderItemID uint64) (*domain.Review, error)
	ListByProduct(productID uint64, req *domain.ReviewListRequest) ([]*domain.Review, int64, error)
	ListByUser(userID uint64, req *domain.ReviewListRequest) ([]*domain.Review, int64, error)
	ListBySeller(sellerID uint64, req *domain.ReviewListRequest) ([]*domain.Review, int64, error)

	// 통계
	GetProductSummary(productID uint64) (*domain.ReviewSummary, error)
	UpdateProductRating(productID uint64) error

	// 도움됨
	IncrementHelpfulCount(id uint64) error
	DecrementHelpfulCount(id uint64) error
}

// ReviewHelpfulRepository 리뷰 도움됨 저장소 인터페이스
type ReviewHelpfulRepository interface {
	Create(helpful *domain.ReviewHelpful) error
	Delete(reviewID, userID uint64) error
	FindByReviewAndUser(reviewID, userID uint64) (*domain.ReviewHelpful, error)
	CountByReview(reviewID uint64) (int64, error)
}

// reviewRepository GORM 구현체
type reviewRepository struct {
	db *gorm.DB
}

// NewReviewRepository 생성자
func NewReviewRepository(db *gorm.DB) ReviewRepository {
	return &reviewRepository{db: db}
}

// Create 리뷰 생성
func (r *reviewRepository) Create(review *domain.Review) error {
	now := time.Now()
	review.CreatedAt = now
	review.UpdatedAt = now
	return r.db.Create(review).Error
}

// Update 리뷰 수정
func (r *reviewRepository) Update(id uint64, review *domain.Review) error {
	review.UpdatedAt = time.Now()
	return r.db.Model(&domain.Review{}).Where("id = ?", id).Updates(review).Error
}

// Delete 리뷰 소프트 삭제
func (r *reviewRepository) Delete(id uint64) error {
	return r.db.Delete(&domain.Review{}, id).Error
}

// UpdateStatus 리뷰 상태 업데이트
func (r *reviewRepository) UpdateStatus(id uint64, status domain.ReviewStatus) error {
	return r.db.Model(&domain.Review{}).Where("id = ?", id).Update("status", status).Error
}

// UpdateSellerReply 판매자 답글 업데이트
func (r *reviewRepository) UpdateSellerReply(id uint64, reply string) error {
	return r.db.Model(&domain.Review{}).Where("id = ?", id).Updates(map[string]interface{}{
		"seller_reply": reply,
		"replied_at":   time.Now(),
		"updated_at":   time.Now(),
	}).Error
}

// FindByID ID로 리뷰 조회
func (r *reviewRepository) FindByID(id uint64) (*domain.Review, error) {
	var review domain.Review
	err := r.db.Where("id = ?", id).First(&review).Error
	if err != nil {
		return nil, err
	}
	return &review, nil
}

// FindByUserAndOrderItem 사용자와 주문 아이템으로 리뷰 조회
func (r *reviewRepository) FindByUserAndOrderItem(userID, orderItemID uint64) (*domain.Review, error) {
	var review domain.Review
	err := r.db.Where("user_id = ? AND order_item_id = ?", userID, orderItemID).First(&review).Error
	if err != nil {
		return nil, err
	}
	return &review, nil
}

// ListByProduct 상품별 리뷰 목록 조회
func (r *reviewRepository) ListByProduct(productID uint64, req *domain.ReviewListRequest) ([]*domain.Review, int64, error) {
	var reviews []*domain.Review
	var total int64

	query := r.db.Model(&domain.Review{}).Where("product_id = ?", productID).Where("status = ?", domain.ReviewStatusApproved)

	// 평점 필터
	if req.Rating != nil {
		query = query.Where("rating = ?", *req.Rating)
	}

	// 전체 카운트
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 정렬
	sortBy := req.SortBy
	if sortBy == "" {
		sortBy = "created_at"
	}
	sortOrder := req.SortOrder
	if sortOrder == "" {
		sortOrder = "desc"
	}
	query = query.Order(sortBy + " " + sortOrder)

	// 페이지네이션
	page := req.Page
	if page < 1 {
		page = 1
	}
	limit := req.Limit
	if limit < 1 {
		limit = 20
	}
	offset := (page - 1) * limit

	err := query.Offset(offset).Limit(limit).Find(&reviews).Error
	if err != nil {
		return nil, 0, err
	}

	return reviews, total, nil
}

// ListByUser 사용자별 리뷰 목록 조회
func (r *reviewRepository) ListByUser(userID uint64, req *domain.ReviewListRequest) ([]*domain.Review, int64, error) {
	var reviews []*domain.Review
	var total int64

	query := r.db.Model(&domain.Review{}).Where("user_id = ?", userID)

	// 전체 카운트
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 정렬
	sortBy := req.SortBy
	if sortBy == "" {
		sortBy = "created_at"
	}
	sortOrder := req.SortOrder
	if sortOrder == "" {
		sortOrder = "desc"
	}
	query = query.Order(sortBy + " " + sortOrder)

	// 페이지네이션
	page := req.Page
	if page < 1 {
		page = 1
	}
	limit := req.Limit
	if limit < 1 {
		limit = 20
	}
	offset := (page - 1) * limit

	err := query.Offset(offset).Limit(limit).Find(&reviews).Error
	if err != nil {
		return nil, 0, err
	}

	return reviews, total, nil
}

// ListBySeller 판매자별 리뷰 목록 조회 (해당 판매자의 상품에 대한 리뷰)
func (r *reviewRepository) ListBySeller(sellerID uint64, req *domain.ReviewListRequest) ([]*domain.Review, int64, error) {
	var reviews []*domain.Review
	var total int64

	query := r.db.Model(&domain.Review{}).
		Joins("JOIN commerce_products ON commerce_reviews.product_id = commerce_products.id").
		Where("commerce_products.seller_id = ?", sellerID)

	// 전체 카운트
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 정렬
	sortBy := req.SortBy
	if sortBy == "" {
		sortBy = "commerce_reviews.created_at"
	}
	sortOrder := req.SortOrder
	if sortOrder == "" {
		sortOrder = "desc"
	}
	query = query.Order(sortBy + " " + sortOrder)

	// 페이지네이션
	page := req.Page
	if page < 1 {
		page = 1
	}
	limit := req.Limit
	if limit < 1 {
		limit = 20
	}
	offset := (page - 1) * limit

	err := query.Offset(offset).Limit(limit).Find(&reviews).Error
	if err != nil {
		return nil, 0, err
	}

	return reviews, total, nil
}

// GetProductSummary 상품 리뷰 요약 조회
func (r *reviewRepository) GetProductSummary(productID uint64) (*domain.ReviewSummary, error) {
	var summary domain.ReviewSummary
	summary.ProductID = productID
	summary.RatingCounts = make(map[uint8]int64)

	// 전체 카운트 및 평균 평점
	var result struct {
		TotalCount    int64
		AverageRating float64
	}
	err := r.db.Model(&domain.Review{}).
		Where("product_id = ? AND status = ?", productID, domain.ReviewStatusApproved).
		Select("COUNT(*) as total_count, COALESCE(AVG(rating), 0) as average_rating").
		Scan(&result).Error
	if err != nil {
		return nil, err
	}
	summary.TotalCount = result.TotalCount
	summary.AverageRating = result.AverageRating

	// 평점별 카운트
	var ratingCounts []struct {
		Rating uint8
		Count  int64
	}
	err = r.db.Model(&domain.Review{}).
		Where("product_id = ? AND status = ?", productID, domain.ReviewStatusApproved).
		Select("rating, COUNT(*) as count").
		Group("rating").
		Scan(&ratingCounts).Error
	if err != nil {
		return nil, err
	}

	for _, rc := range ratingCounts {
		summary.RatingCounts[rc.Rating] = rc.Count
	}

	return &summary, nil
}

// UpdateProductRating 상품의 평균 평점 및 리뷰 수 업데이트
func (r *reviewRepository) UpdateProductRating(productID uint64) error {
	var result struct {
		RatingAvg   float64
		RatingCount int64
	}
	err := r.db.Model(&domain.Review{}).
		Where("product_id = ? AND status = ?", productID, domain.ReviewStatusApproved).
		Select("COALESCE(AVG(rating), 0) as rating_avg, COUNT(*) as rating_count").
		Scan(&result).Error
	if err != nil {
		return err
	}

	return r.db.Model(&domain.Product{}).Where("id = ?", productID).Updates(map[string]interface{}{
		"rating_avg":   result.RatingAvg,
		"rating_count": result.RatingCount,
		"updated_at":   time.Now(),
	}).Error
}

// IncrementHelpfulCount 도움됨 카운트 증가
func (r *reviewRepository) IncrementHelpfulCount(id uint64) error {
	return r.db.Model(&domain.Review{}).
		Where("id = ?", id).
		UpdateColumn("helpful_count", gorm.Expr("helpful_count + 1")).Error
}

// DecrementHelpfulCount 도움됨 카운트 감소
func (r *reviewRepository) DecrementHelpfulCount(id uint64) error {
	return r.db.Model(&domain.Review{}).
		Where("id = ? AND helpful_count > 0", id).
		UpdateColumn("helpful_count", gorm.Expr("helpful_count - 1")).Error
}

// reviewHelpfulRepository GORM 구현체
type reviewHelpfulRepository struct {
	db *gorm.DB
}

// NewReviewHelpfulRepository 생성자
func NewReviewHelpfulRepository(db *gorm.DB) ReviewHelpfulRepository {
	return &reviewHelpfulRepository{db: db}
}

// Create 도움됨 생성
func (r *reviewHelpfulRepository) Create(helpful *domain.ReviewHelpful) error {
	helpful.CreatedAt = time.Now()
	return r.db.Create(helpful).Error
}

// Delete 도움됨 삭제
func (r *reviewHelpfulRepository) Delete(reviewID, userID uint64) error {
	return r.db.Where("review_id = ? AND user_id = ?", reviewID, userID).Delete(&domain.ReviewHelpful{}).Error
}

// FindByReviewAndUser 리뷰와 사용자로 도움됨 조회
func (r *reviewHelpfulRepository) FindByReviewAndUser(reviewID, userID uint64) (*domain.ReviewHelpful, error) {
	var helpful domain.ReviewHelpful
	err := r.db.Where("review_id = ? AND user_id = ?", reviewID, userID).First(&helpful).Error
	if err != nil {
		return nil, err
	}
	return &helpful, nil
}

// CountByReview 리뷰의 도움됨 카운트
func (r *reviewHelpfulRepository) CountByReview(reviewID uint64) (int64, error) {
	var count int64
	err := r.db.Model(&domain.ReviewHelpful{}).Where("review_id = ?", reviewID).Count(&count).Error
	return count, err
}
