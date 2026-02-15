package repository

import (
	"time"

	"github.com/damoang/angple-backend/internal/plugins/commerce/domain"
	"gorm.io/gorm"
)

// CouponRepository 쿠폰 저장소 인터페이스
type CouponRepository interface {
	// 생성/수정/삭제
	Create(coupon *domain.Coupon) error
	Update(id uint64, coupon *domain.Coupon) error
	Delete(id uint64) error

	// 조회
	FindByID(id uint64) (*domain.Coupon, error)
	FindByCode(code string) (*domain.Coupon, error)
	List(req *domain.CouponListRequest) ([]*domain.Coupon, int64, error)
	ListPublicActive() ([]*domain.Coupon, error)

	// 사용 횟수 관리
	IncrementUsageCount(id uint64) error
	DecrementUsageCount(id uint64) error

	// 만료 처리
	UpdateExpiredCoupons() (int64, error)
}

// CouponUsageRepository 쿠폰 사용 내역 저장소 인터페이스
type CouponUsageRepository interface {
	Create(usage *domain.CouponUsage) error
	FindByOrderID(orderID uint64) (*domain.CouponUsage, error)
	CountByUserAndCoupon(userID, couponID uint64) (int64, error)
	ListByUserID(userID uint64) ([]*domain.CouponUsage, error)
	DeleteByOrderID(orderID uint64) error
}

// couponRepository GORM 구현체
type couponRepository struct {
	db *gorm.DB
}

// NewCouponRepository 생성자
func NewCouponRepository(db *gorm.DB) CouponRepository {
	return &couponRepository{db: db}
}

// Create 쿠폰 생성
func (r *couponRepository) Create(coupon *domain.Coupon) error {
	now := time.Now()
	coupon.CreatedAt = now
	coupon.UpdatedAt = now
	return r.db.Create(coupon).Error
}

// Update 쿠폰 수정
func (r *couponRepository) Update(id uint64, coupon *domain.Coupon) error {
	coupon.UpdatedAt = time.Now()
	return r.db.Model(&domain.Coupon{}).Where("id = ?", id).Updates(coupon).Error
}

// Delete 쿠폰 소프트 삭제
func (r *couponRepository) Delete(id uint64) error {
	return r.db.Delete(&domain.Coupon{}, id).Error
}

// FindByID ID로 쿠폰 조회
func (r *couponRepository) FindByID(id uint64) (*domain.Coupon, error) {
	var coupon domain.Coupon
	err := r.db.Where("id = ?", id).First(&coupon).Error
	if err != nil {
		return nil, err
	}
	return &coupon, nil
}

// FindByCode 코드로 쿠폰 조회
func (r *couponRepository) FindByCode(code string) (*domain.Coupon, error) {
	var coupon domain.Coupon
	err := r.db.Where("code = ?", code).First(&coupon).Error
	if err != nil {
		return nil, err
	}
	return &coupon, nil
}

// List 쿠폰 목록 조회
func (r *couponRepository) List(req *domain.CouponListRequest) ([]*domain.Coupon, int64, error) {
	var coupons []*domain.Coupon
	var total int64

	query := r.db.Model(&domain.Coupon{})

	// 상태 필터
	if req.Status != "" {
		query = query.Where("status = ?", req.Status)
	}

	// 공개 여부 필터
	if req.IsPublic != nil {
		query = query.Where("is_public = ?", *req.IsPublic)
	}

	// 전체 카운트
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

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

	err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&coupons).Error
	if err != nil {
		return nil, 0, err
	}

	return coupons, total, nil
}

// ListPublicActive 공개된 활성 쿠폰 목록 조회
func (r *couponRepository) ListPublicActive() ([]*domain.Coupon, error) {
	var coupons []*domain.Coupon
	now := time.Now()

	err := r.db.Where("is_public = ?", true).
		Where("status = ?", domain.CouponStatusActive).
		Where("(starts_at IS NULL OR starts_at <= ?)", now).
		Where("(expires_at IS NULL OR expires_at > ?)", now).
		Where("(usage_limit IS NULL OR usage_count < usage_limit)").
		Order("created_at DESC").
		Find(&coupons).Error
	if err != nil {
		return nil, err
	}
	return coupons, nil
}

// IncrementUsageCount 사용 횟수 증가
func (r *couponRepository) IncrementUsageCount(id uint64) error {
	return r.db.Model(&domain.Coupon{}).
		Where("id = ?", id).
		UpdateColumn("usage_count", gorm.Expr("usage_count + 1")).Error
}

// DecrementUsageCount 사용 횟수 감소
func (r *couponRepository) DecrementUsageCount(id uint64) error {
	return r.db.Model(&domain.Coupon{}).
		Where("id = ? AND usage_count > 0", id).
		UpdateColumn("usage_count", gorm.Expr("usage_count - 1")).Error
}

// UpdateExpiredCoupons 만료된 쿠폰 상태 업데이트
func (r *couponRepository) UpdateExpiredCoupons() (int64, error) {
	result := r.db.Model(&domain.Coupon{}).
		Where("status = ?", domain.CouponStatusActive).
		Where("expires_at IS NOT NULL AND expires_at < ?", time.Now()).
		Update("status", domain.CouponStatusExpired)
	return result.RowsAffected, result.Error
}

// couponUsageRepository GORM 구현체
type couponUsageRepository struct {
	db *gorm.DB
}

// NewCouponUsageRepository 생성자
func NewCouponUsageRepository(db *gorm.DB) CouponUsageRepository {
	return &couponUsageRepository{db: db}
}

// Create 쿠폰 사용 내역 생성
func (r *couponUsageRepository) Create(usage *domain.CouponUsage) error {
	usage.UsedAt = time.Now()
	return r.db.Create(usage).Error
}

// FindByOrderID 주문 ID로 쿠폰 사용 내역 조회
func (r *couponUsageRepository) FindByOrderID(orderID uint64) (*domain.CouponUsage, error) {
	var usage domain.CouponUsage
	err := r.db.Where("order_id = ?", orderID).First(&usage).Error
	if err != nil {
		return nil, err
	}
	return &usage, nil
}

// CountByUserAndCoupon 사용자의 특정 쿠폰 사용 횟수 조회
func (r *couponUsageRepository) CountByUserAndCoupon(userID, couponID uint64) (int64, error) {
	var count int64
	err := r.db.Model(&domain.CouponUsage{}).
		Where("user_id = ? AND coupon_id = ?", userID, couponID).
		Count(&count).Error
	return count, err
}

// ListByUserID 사용자의 쿠폰 사용 내역 조회
func (r *couponUsageRepository) ListByUserID(userID uint64) ([]*domain.CouponUsage, error) {
	var usages []*domain.CouponUsage
	err := r.db.Preload("Coupon").
		Where("user_id = ?", userID).
		Order("used_at DESC").
		Find(&usages).Error
	if err != nil {
		return nil, err
	}
	return usages, nil
}

// DeleteByOrderID 주문 ID로 쿠폰 사용 내역 삭제
func (r *couponUsageRepository) DeleteByOrderID(orderID uint64) error {
	return r.db.Where("order_id = ?", orderID).Delete(&domain.CouponUsage{}).Error
}
