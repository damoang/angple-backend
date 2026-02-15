package service

import (
	"errors"
	"strings"
	"time"

	"github.com/damoang/angple-backend/internal/plugins/commerce/domain"
	"github.com/damoang/angple-backend/internal/plugins/commerce/repository"
	"gorm.io/gorm"
)

// 쿠폰 에러 정의
var (
	ErrCouponNotFound       = errors.New("coupon not found")
	ErrCouponCodeExists     = errors.New("coupon code already exists")
	ErrCouponExpired        = errors.New("coupon has expired")
	ErrCouponNotStarted     = errors.New("coupon is not yet valid")
	ErrCouponInactive       = errors.New("coupon is inactive")
	ErrCouponUsageLimitHit  = errors.New("coupon usage limit reached")
	ErrCouponUserLimitHit   = errors.New("you have already used this coupon the maximum number of times")
	ErrCouponMinOrderAmount = errors.New("order amount is below minimum required")
	ErrCouponAlreadyApplied = errors.New("coupon already applied to this order")
	ErrCouponNotApplicable  = errors.New("coupon is not applicable to this order")
	ErrInvalidDiscountType  = errors.New("invalid discount type")
)

// CouponService 쿠폰 서비스 인터페이스
type CouponService interface {
	// 관리자 기능
	CreateCoupon(adminID uint64, req *domain.CreateCouponRequest) (*domain.CouponResponse, error)
	UpdateCoupon(couponID uint64, req *domain.UpdateCouponRequest) (*domain.CouponResponse, error)
	DeleteCoupon(couponID uint64) error
	GetCoupon(couponID uint64) (*domain.CouponResponse, error)
	ListCoupons(req *domain.CouponListRequest) ([]*domain.CouponResponse, int64, error)

	// 사용자 기능
	ValidateCoupon(userID uint64, code string, orderAmount float64) (*domain.ValidateCouponResponse, error)
	GetPublicCoupons() ([]*domain.CouponResponse, error)
	ApplyCoupon(userID uint64, orderID uint64, code string) (float64, error)
	RemoveCoupon(orderID uint64) error

	// 유틸리티
	CalculateDiscount(coupon *domain.Coupon, orderAmount float64) float64
}

// couponService 구현체
type couponService struct {
	couponRepo      repository.CouponRepository
	couponUsageRepo repository.CouponUsageRepository
	orderRepo       repository.OrderRepository
}

// NewCouponService 생성자
func NewCouponService(
	couponRepo repository.CouponRepository,
	couponUsageRepo repository.CouponUsageRepository,
	orderRepo repository.OrderRepository,
) CouponService {
	return &couponService{
		couponRepo:      couponRepo,
		couponUsageRepo: couponUsageRepo,
		orderRepo:       orderRepo,
	}
}

// CreateCoupon 쿠폰 생성
func (s *couponService) CreateCoupon(adminID uint64, req *domain.CreateCouponRequest) (*domain.CouponResponse, error) {
	// 코드 중복 검사
	code := strings.ToUpper(strings.TrimSpace(req.Code))
	existing, err := s.couponRepo.FindByCode(code)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if existing != nil {
		return nil, ErrCouponCodeExists
	}

	// 할인 타입 검증
	discountType := domain.DiscountType(req.DiscountType)
	if discountType != domain.DiscountTypeFixed &&
		discountType != domain.DiscountTypePercent &&
		discountType != domain.DiscountTypeFreeShipping {
		return nil, ErrInvalidDiscountType
	}

	// 적용 대상 설정
	applyTo := domain.ApplyToAll
	if req.ApplyTo != "" {
		applyTo = domain.ApplyTo(req.ApplyTo)
	}

	coupon := &domain.Coupon{
		Code:           code,
		Name:           req.Name,
		Description:    req.Description,
		DiscountType:   discountType,
		DiscountValue:  req.DiscountValue,
		MaxDiscount:    req.MaxDiscount,
		MinOrderAmount: req.MinOrderAmount,
		ApplyTo:        applyTo,
		UsageLimit:     req.UsageLimit,
		UsagePerUser:   req.UsagePerUser,
		Status:         domain.CouponStatusActive,
		IsPublic:       req.IsPublic,
		CreatedBy:      &adminID,
	}

	if coupon.UsagePerUser == 0 {
		coupon.UsagePerUser = 1
	}

	// 적용 대상 ID 설정
	if len(req.ApplyIDs) > 0 {
		if err := coupon.SetApplyIDs(req.ApplyIDs); err != nil {
			return nil, err
		}
	}

	// 시작일 파싱
	if req.StartsAt != "" {
		startsAt, err := time.Parse(time.RFC3339, req.StartsAt)
		if err != nil {
			return nil, errors.New("invalid starts_at format")
		}
		coupon.StartsAt = &startsAt
	}

	// 만료일 파싱
	if req.ExpiresAt != "" {
		expiresAt, err := time.Parse(time.RFC3339, req.ExpiresAt)
		if err != nil {
			return nil, errors.New("invalid expires_at format")
		}
		coupon.ExpiresAt = &expiresAt
	}

	if err := s.couponRepo.Create(coupon); err != nil {
		return nil, err
	}

	return coupon.ToResponse(), nil
}

// UpdateCoupon 쿠폰 수정
func (s *couponService) UpdateCoupon(couponID uint64, req *domain.UpdateCouponRequest) (*domain.CouponResponse, error) {
	coupon, err := s.couponRepo.FindByID(couponID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCouponNotFound
		}
		return nil, err
	}

	// 업데이트 적용
	if req.Name != nil {
		coupon.Name = *req.Name
	}
	if req.Description != nil {
		coupon.Description = *req.Description
	}
	if req.DiscountValue != nil {
		coupon.DiscountValue = *req.DiscountValue
	}
	if req.MaxDiscount != nil {
		coupon.MaxDiscount = req.MaxDiscount
	}
	if req.MinOrderAmount != nil {
		coupon.MinOrderAmount = *req.MinOrderAmount
	}
	if req.UsageLimit != nil {
		coupon.UsageLimit = req.UsageLimit
	}
	if req.UsagePerUser != nil {
		coupon.UsagePerUser = *req.UsagePerUser
	}
	if req.Status != nil {
		coupon.Status = domain.CouponStatus(*req.Status)
	}
	if req.IsPublic != nil {
		coupon.IsPublic = *req.IsPublic
	}

	// 시작일 파싱
	if req.StartsAt != nil {
		if *req.StartsAt == "" {
			coupon.StartsAt = nil
		} else {
			startsAt, err := time.Parse(time.RFC3339, *req.StartsAt)
			if err != nil {
				return nil, errors.New("invalid starts_at format")
			}
			coupon.StartsAt = &startsAt
		}
	}

	// 만료일 파싱
	if req.ExpiresAt != nil {
		if *req.ExpiresAt == "" {
			coupon.ExpiresAt = nil
		} else {
			expiresAt, err := time.Parse(time.RFC3339, *req.ExpiresAt)
			if err != nil {
				return nil, errors.New("invalid expires_at format")
			}
			coupon.ExpiresAt = &expiresAt
		}
	}

	if err := s.couponRepo.Update(couponID, coupon); err != nil {
		return nil, err
	}

	return coupon.ToResponse(), nil
}

// DeleteCoupon 쿠폰 삭제
func (s *couponService) DeleteCoupon(couponID uint64) error {
	_, err := s.couponRepo.FindByID(couponID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCouponNotFound
		}
		return err
	}
	return s.couponRepo.Delete(couponID)
}

// GetCoupon 쿠폰 조회
func (s *couponService) GetCoupon(couponID uint64) (*domain.CouponResponse, error) {
	coupon, err := s.couponRepo.FindByID(couponID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCouponNotFound
		}
		return nil, err
	}
	return coupon.ToResponse(), nil
}

// ListCoupons 쿠폰 목록 조회
func (s *couponService) ListCoupons(req *domain.CouponListRequest) ([]*domain.CouponResponse, int64, error) {
	coupons, total, err := s.couponRepo.List(req)
	if err != nil {
		return nil, 0, err
	}

	responses := make([]*domain.CouponResponse, 0, len(coupons))
	for _, coupon := range coupons {
		responses = append(responses, coupon.ToResponse())
	}

	return responses, total, nil
}

// ValidateCoupon 쿠폰 유효성 검증
func (s *couponService) ValidateCoupon(userID uint64, code string, orderAmount float64) (*domain.ValidateCouponResponse, error) {
	code = strings.ToUpper(strings.TrimSpace(code))

	coupon, err := s.couponRepo.FindByCode(code)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &domain.ValidateCouponResponse{
				Valid:   false,
				Code:    code,
				Message: "쿠폰을 찾을 수 없습니다",
			}, nil
		}
		return nil, err
	}

	// 상태 검사
	if coupon.Status != domain.CouponStatusActive {
		return &domain.ValidateCouponResponse{
			Valid:   false,
			Code:    code,
			Name:    coupon.Name,
			Message: "비활성화된 쿠폰입니다",
		}, nil
	}

	// 시작일 검사
	now := time.Now()
	if coupon.StartsAt != nil && now.Before(*coupon.StartsAt) {
		return &domain.ValidateCouponResponse{
			Valid:   false,
			Code:    code,
			Name:    coupon.Name,
			Message: "아직 사용 기간이 아닙니다",
		}, nil
	}

	// 만료일 검사
	if coupon.ExpiresAt != nil && now.After(*coupon.ExpiresAt) {
		return &domain.ValidateCouponResponse{
			Valid:   false,
			Code:    code,
			Name:    coupon.Name,
			Message: "만료된 쿠폰입니다",
		}, nil
	}

	// 총 사용 제한 검사
	if coupon.UsageLimit != nil && coupon.UsageCount >= *coupon.UsageLimit {
		return &domain.ValidateCouponResponse{
			Valid:   false,
			Code:    code,
			Name:    coupon.Name,
			Message: "쿠폰 사용 한도가 초과되었습니다",
		}, nil
	}

	// 사용자별 사용 제한 검사
	userUsageCount, err := s.couponUsageRepo.CountByUserAndCoupon(userID, coupon.ID)
	if err != nil {
		return nil, err
	}
	if uint(userUsageCount) >= coupon.UsagePerUser {
		return &domain.ValidateCouponResponse{
			Valid:   false,
			Code:    code,
			Name:    coupon.Name,
			Message: "이미 최대 사용 횟수에 도달했습니다",
		}, nil
	}

	// 최소 주문 금액 검사
	if orderAmount < coupon.MinOrderAmount {
		return &domain.ValidateCouponResponse{
			Valid:          false,
			Code:           code,
			Name:           coupon.Name,
			MinOrderAmount: coupon.MinOrderAmount,
			Message:        "최소 주문 금액을 충족하지 않습니다",
		}, nil
	}

	return &domain.ValidateCouponResponse{
		Valid:          true,
		Code:           coupon.Code,
		Name:           coupon.Name,
		DiscountType:   string(coupon.DiscountType),
		DiscountValue:  coupon.DiscountValue,
		MaxDiscount:    coupon.MaxDiscount,
		MinOrderAmount: coupon.MinOrderAmount,
		Message:        "사용 가능한 쿠폰입니다",
	}, nil
}

// GetPublicCoupons 공개 쿠폰 목록 조회
func (s *couponService) GetPublicCoupons() ([]*domain.CouponResponse, error) {
	coupons, err := s.couponRepo.ListPublicActive()
	if err != nil {
		return nil, err
	}

	responses := make([]*domain.CouponResponse, 0, len(coupons))
	for _, coupon := range coupons {
		responses = append(responses, coupon.ToResponse())
	}

	return responses, nil
}

// ApplyCoupon 주문에 쿠폰 적용
func (s *couponService) ApplyCoupon(userID uint64, orderID uint64, code string) (float64, error) {
	code = strings.ToUpper(strings.TrimSpace(code))

	// 주문 조회
	order, err := s.orderRepo.FindByID(orderID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, ErrOrderNotFound
		}
		return 0, err
	}

	// 소유자 확인
	if order.UserID != userID {
		return 0, ErrOrderForbidden
	}

	// 주문 상태 확인 (결제 대기 상태만)
	if order.Status != domain.OrderStatusPending {
		return 0, errors.New("cannot apply coupon to this order")
	}

	// 이미 쿠폰이 적용되어 있는지 확인
	existingUsage, err := s.couponUsageRepo.FindByOrderID(orderID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}
	if existingUsage != nil {
		return 0, ErrCouponAlreadyApplied
	}

	// 쿠폰 유효성 검증
	validateResult, err := s.ValidateCoupon(userID, code, order.Subtotal)
	if err != nil {
		return 0, err
	}
	if !validateResult.Valid {
		return 0, errors.New(validateResult.Message)
	}

	// 쿠폰 조회
	coupon, err := s.couponRepo.FindByCode(code)
	if err != nil {
		return 0, err
	}

	// 할인 금액 계산
	discountAmount := s.CalculateDiscount(coupon, order.Subtotal)
	if discountAmount == 0 && coupon.DiscountType != domain.DiscountTypeFreeShipping {
		return 0, ErrCouponNotApplicable
	}

	// 쿠폰 사용 내역 생성
	usage := &domain.CouponUsage{
		CouponID:       coupon.ID,
		UserID:         userID,
		OrderID:        orderID,
		DiscountAmount: discountAmount,
	}
	if err := s.couponUsageRepo.Create(usage); err != nil {
		return 0, err
	}

	// 쿠폰 사용 횟수 증가
	if err := s.couponRepo.IncrementUsageCount(coupon.ID); err != nil {
		return 0, err
	}

	// 주문 할인 금액 업데이트
	order.Discount = discountAmount
	order.Total = order.Subtotal - discountAmount + order.ShippingFee
	if order.Total < 0 {
		order.Total = 0
	}

	if err := s.orderRepo.Update(order.ID, order); err != nil {
		return 0, err
	}

	return discountAmount, nil
}

// RemoveCoupon 주문에서 쿠폰 제거
func (s *couponService) RemoveCoupon(orderID uint64) error {
	// 쿠폰 사용 내역 조회
	usage, err := s.couponUsageRepo.FindByOrderID(orderID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil // 적용된 쿠폰이 없음
		}
		return err
	}

	// 주문 조회
	order, err := s.orderRepo.FindByID(orderID)
	if err != nil {
		return err
	}

	// 주문 상태 확인 (결제 대기 상태만)
	if order.Status != domain.OrderStatusPending {
		return errors.New("cannot remove coupon from this order")
	}

	// 쿠폰 사용 횟수 감소
	if err := s.couponRepo.DecrementUsageCount(usage.CouponID); err != nil {
		return err
	}

	// 쿠폰 사용 내역 삭제
	if err := s.couponUsageRepo.DeleteByOrderID(orderID); err != nil {
		return err
	}

	// 주문 할인 금액 초기화
	order.Discount = 0
	order.Total = order.Subtotal + order.ShippingFee

	return s.orderRepo.Update(order.ID, order)
}

// CalculateDiscount 할인 금액 계산
func (s *couponService) CalculateDiscount(coupon *domain.Coupon, orderAmount float64) float64 {
	return coupon.CalculateDiscount(orderAmount)
}
