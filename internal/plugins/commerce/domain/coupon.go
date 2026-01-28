package domain

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// DiscountType 할인 유형
type DiscountType string

const (
	DiscountTypeFixed        DiscountType = "fixed"         // 정액 할인
	DiscountTypePercent      DiscountType = "percent"       // 정률 할인
	DiscountTypeFreeShipping DiscountType = "free_shipping" // 무료 배송
)

// ApplyTo 쿠폰 적용 대상
type ApplyTo string

const (
	ApplyToAll      ApplyTo = "all"      // 모든 상품
	ApplyToProduct  ApplyTo = "product"  // 특정 상품
	ApplyToCategory ApplyTo = "category" // 특정 카테고리
	ApplyToSeller   ApplyTo = "seller"   // 특정 판매자
)

// CouponStatus 쿠폰 상태
type CouponStatus string

const (
	CouponStatusActive   CouponStatus = "active"
	CouponStatusInactive CouponStatus = "inactive"
	CouponStatusExpired  CouponStatus = "expired"
)

// Coupon 쿠폰 엔티티
type Coupon struct {
	ID          uint64 `gorm:"primaryKey" json:"id"`
	Code        string `gorm:"column:code;size:50;uniqueIndex;not null" json:"code"`
	Name        string `gorm:"column:name;size:100;not null" json:"name"`
	Description string `gorm:"type:text" json:"description,omitempty"`

	// 할인 정보
	DiscountType  DiscountType `gorm:"column:discount_type;not null" json:"discount_type"`
	DiscountValue float64      `gorm:"column:discount_value;type:decimal(12,2);not null" json:"discount_value"`
	MaxDiscount   *float64     `gorm:"column:max_discount;type:decimal(12,2)" json:"max_discount,omitempty"`

	// 사용 조건
	MinOrderAmount float64 `gorm:"column:min_order_amount;type:decimal(12,2);default:0" json:"min_order_amount"`

	// 적용 범위
	ApplyTo  ApplyTo `gorm:"column:apply_to;default:'all'" json:"apply_to"`
	ApplyIDs string  `gorm:"column:apply_ids;type:json" json:"-"`

	// 사용 제한
	UsageLimit   *uint `gorm:"column:usage_limit" json:"usage_limit,omitempty"`
	UsagePerUser uint  `gorm:"column:usage_per_user;default:1" json:"usage_per_user"`
	UsageCount   uint  `gorm:"column:usage_count;default:0" json:"usage_count"`

	// 유효 기간
	StartsAt  *time.Time `gorm:"column:starts_at" json:"starts_at,omitempty"`
	ExpiresAt *time.Time `gorm:"column:expires_at" json:"expires_at,omitempty"`

	// 상태
	Status   CouponStatus `gorm:"column:status;default:'active'" json:"status"`
	IsPublic bool         `gorm:"column:is_public;default:false" json:"is_public"`

	// 생성자
	CreatedBy *uint64 `gorm:"column:created_by" json:"created_by,omitempty"`

	// 타임스탬프
	CreatedAt time.Time      `gorm:"column:created_at" json:"created_at"`
	UpdatedAt time.Time      `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index" json:"-"`
}

// TableName GORM 테이블명
func (Coupon) TableName() string {
	return "commerce_coupons"
}

// GetApplyIDs ApplyIDs JSON을 파싱하여 uint64 슬라이스로 반환
func (c *Coupon) GetApplyIDs() []uint64 {
	if c.ApplyIDs == "" || c.ApplyIDs == "null" {
		return nil
	}
	var ids []uint64
	if err := json.Unmarshal([]byte(c.ApplyIDs), &ids); err != nil {
		return nil
	}
	return ids
}

// SetApplyIDs uint64 슬라이스를 ApplyIDs JSON으로 설정
func (c *Coupon) SetApplyIDs(ids []uint64) error {
	if len(ids) == 0 {
		c.ApplyIDs = ""
		return nil
	}
	data, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	c.ApplyIDs = string(data)
	return nil
}

// IsValid 쿠폰이 유효한지 확인
func (c *Coupon) IsValid() bool {
	if c.Status != CouponStatusActive {
		return false
	}

	now := time.Now()
	if c.StartsAt != nil && now.Before(*c.StartsAt) {
		return false
	}
	if c.ExpiresAt != nil && now.After(*c.ExpiresAt) {
		return false
	}
	if c.UsageLimit != nil && c.UsageCount >= *c.UsageLimit {
		return false
	}
	return true
}

// CalculateDiscount 할인 금액 계산
func (c *Coupon) CalculateDiscount(orderAmount float64) float64 {
	if orderAmount < c.MinOrderAmount {
		return 0
	}

	var discount float64
	switch c.DiscountType {
	case DiscountTypeFixed:
		discount = c.DiscountValue
	case DiscountTypePercent:
		discount = orderAmount * (c.DiscountValue / 100)
		if c.MaxDiscount != nil && discount > *c.MaxDiscount {
			discount = *c.MaxDiscount
		}
	case DiscountTypeFreeShipping:
		return 0 // 배송비 할인은 별도 처리
	}

	// 할인 금액이 주문 금액을 초과할 수 없음
	if discount > orderAmount {
		discount = orderAmount
	}
	return discount
}

// CouponUsage 쿠폰 사용 내역
type CouponUsage struct {
	ID             uint64    `gorm:"primaryKey" json:"id"`
	CouponID       uint64    `gorm:"column:coupon_id;not null" json:"coupon_id"`
	UserID         uint64    `gorm:"column:user_id;not null" json:"user_id"`
	OrderID        uint64    `gorm:"column:order_id;not null" json:"order_id"`
	DiscountAmount float64   `gorm:"column:discount_amount;type:decimal(12,2);not null" json:"discount_amount"`
	UsedAt         time.Time `gorm:"column:used_at" json:"used_at"`

	// Relations
	Coupon *Coupon `gorm:"foreignKey:CouponID" json:"-"`
	Order  *Order  `gorm:"foreignKey:OrderID" json:"-"`
}

// TableName GORM 테이블명
func (CouponUsage) TableName() string {
	return "commerce_coupon_usages"
}

// CreateCouponRequest 쿠폰 생성 요청 DTO
type CreateCouponRequest struct {
	Code           string   `json:"code" binding:"required,min=3,max=50"`
	Name           string   `json:"name" binding:"required,min=1,max=100"`
	Description    string   `json:"description" binding:"omitempty,max=1000"`
	DiscountType   string   `json:"discount_type" binding:"required,oneof=fixed percent free_shipping"`
	DiscountValue  float64  `json:"discount_value" binding:"required,gt=0"`
	MaxDiscount    *float64 `json:"max_discount" binding:"omitempty,gt=0"`
	MinOrderAmount float64  `json:"min_order_amount" binding:"omitempty,gte=0"`
	ApplyTo        string   `json:"apply_to" binding:"omitempty,oneof=all product category seller"`
	ApplyIDs       []uint64 `json:"apply_ids" binding:"omitempty"`
	UsageLimit     *uint    `json:"usage_limit" binding:"omitempty,gt=0"`
	UsagePerUser   uint     `json:"usage_per_user" binding:"omitempty,gte=1"`
	StartsAt       string   `json:"starts_at" binding:"omitempty,datetime=2006-01-02T15:04:05Z07:00"`
	ExpiresAt      string   `json:"expires_at" binding:"omitempty,datetime=2006-01-02T15:04:05Z07:00"`
	IsPublic       bool     `json:"is_public"`
}

// UpdateCouponRequest 쿠폰 수정 요청 DTO
type UpdateCouponRequest struct {
	Name           *string  `json:"name" binding:"omitempty,min=1,max=100"`
	Description    *string  `json:"description" binding:"omitempty,max=1000"`
	DiscountValue  *float64 `json:"discount_value" binding:"omitempty,gt=0"`
	MaxDiscount    *float64 `json:"max_discount" binding:"omitempty,gt=0"`
	MinOrderAmount *float64 `json:"min_order_amount" binding:"omitempty,gte=0"`
	UsageLimit     *uint    `json:"usage_limit" binding:"omitempty,gt=0"`
	UsagePerUser   *uint    `json:"usage_per_user" binding:"omitempty,gte=1"`
	StartsAt       *string  `json:"starts_at" binding:"omitempty,datetime=2006-01-02T15:04:05Z07:00"`
	ExpiresAt      *string  `json:"expires_at" binding:"omitempty,datetime=2006-01-02T15:04:05Z07:00"`
	Status         *string  `json:"status" binding:"omitempty,oneof=active inactive"`
	IsPublic       *bool    `json:"is_public"`
}

// ValidateCouponRequest 쿠폰 유효성 검증 요청 DTO
type ValidateCouponRequest struct {
	Code string `json:"code" binding:"required"`
}

// ApplyCouponRequest 쿠폰 적용 요청 DTO
type ApplyCouponRequest struct {
	OrderID uint64 `json:"order_id" binding:"required"`
	Code    string `json:"code" binding:"required"`
}

// CouponResponse 쿠폰 응답 DTO
type CouponResponse struct {
	ID             uint64   `json:"id"`
	Code           string   `json:"code"`
	Name           string   `json:"name"`
	Description    string   `json:"description,omitempty"`
	DiscountType   string   `json:"discount_type"`
	DiscountValue  float64  `json:"discount_value"`
	MaxDiscount    *float64 `json:"max_discount,omitempty"`
	MinOrderAmount float64  `json:"min_order_amount"`
	ApplyTo        string   `json:"apply_to"`
	ApplyIDs       []uint64 `json:"apply_ids,omitempty"`
	UsageLimit     *uint    `json:"usage_limit,omitempty"`
	UsagePerUser   uint     `json:"usage_per_user"`
	UsageCount     uint     `json:"usage_count"`
	StartsAt       string   `json:"starts_at,omitempty"`
	ExpiresAt      string   `json:"expires_at,omitempty"`
	Status         string   `json:"status"`
	IsPublic       bool     `json:"is_public"`
	CreatedAt      string   `json:"created_at"`
}

// ValidateCouponResponse 쿠폰 유효성 검증 응답 DTO
type ValidateCouponResponse struct {
	Valid          bool     `json:"valid"`
	Code           string   `json:"code"`
	Name           string   `json:"name"`
	DiscountType   string   `json:"discount_type"`
	DiscountValue  float64  `json:"discount_value"`
	MaxDiscount    *float64 `json:"max_discount,omitempty"`
	MinOrderAmount float64  `json:"min_order_amount"`
	Message        string   `json:"message,omitempty"`
}

// CouponListRequest 쿠폰 목록 조회 요청
type CouponListRequest struct {
	Page     int    `form:"page" binding:"omitempty,gte=1"`
	Limit    int    `form:"limit" binding:"omitempty,gte=1,lte=100"`
	Status   string `form:"status" binding:"omitempty,oneof=active inactive expired"`
	IsPublic *bool  `form:"is_public"`
}

// ToResponse Coupon을 CouponResponse로 변환
func (c *Coupon) ToResponse() *CouponResponse {
	response := &CouponResponse{
		ID:             c.ID,
		Code:           c.Code,
		Name:           c.Name,
		Description:    c.Description,
		DiscountType:   string(c.DiscountType),
		DiscountValue:  c.DiscountValue,
		MaxDiscount:    c.MaxDiscount,
		MinOrderAmount: c.MinOrderAmount,
		ApplyTo:        string(c.ApplyTo),
		ApplyIDs:       c.GetApplyIDs(),
		UsageLimit:     c.UsageLimit,
		UsagePerUser:   c.UsagePerUser,
		UsageCount:     c.UsageCount,
		Status:         string(c.Status),
		IsPublic:       c.IsPublic,
		CreatedAt:      c.CreatedAt.Format(time.RFC3339),
	}
	if c.StartsAt != nil {
		response.StartsAt = c.StartsAt.Format(time.RFC3339)
	}
	if c.ExpiresAt != nil {
		response.ExpiresAt = c.ExpiresAt.Format(time.RFC3339)
	}
	return response
}
