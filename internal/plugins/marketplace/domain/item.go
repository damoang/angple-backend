package domain

import (
	"time"
)

// ItemStatus 상품 상태
type ItemStatus string

const (
	ItemStatusSelling  ItemStatus = "selling"  // 판매중
	ItemStatusReserved ItemStatus = "reserved" // 예약중
	ItemStatusSold     ItemStatus = "sold"     // 판매완료
	ItemStatusHidden   ItemStatus = "hidden"   // 숨김
)

// TradeMethod 거래 방법
type TradeMethod string

const (
	TradeMethodDirect   TradeMethod = "direct"   // 직거래
	TradeMethodDelivery TradeMethod = "delivery" // 택배
	TradeMethodBoth     TradeMethod = "both"     // 직거래+택배
)

// ItemCondition 상품 상태(컨디션)
type ItemCondition string

const (
	ConditionNew     ItemCondition = "new"      // 새상품
	ConditionLikeNew ItemCondition = "like_new" // 거의 새것
	ConditionGood    ItemCondition = "good"     // 상태 좋음
	ConditionFair    ItemCondition = "fair"     // 사용감 있음
	ConditionPoor    ItemCondition = "poor"     // 상태 나쁨
)

// Item 중고 상품 엔티티
type Item struct {
	ID            uint64        `gorm:"primaryKey" json:"id"`
	SellerID      uint64        `gorm:"column:seller_id;not null;index" json:"seller_id"`
	CategoryID    *uint64       `gorm:"column:category_id;index" json:"category_id,omitempty"`
	Title         string        `gorm:"column:title;size:200;not null" json:"title"`
	Description   string        `gorm:"column:description;type:text" json:"description"`
	Price         int64         `gorm:"column:price;not null" json:"price"`
	OriginalPrice *int64        `gorm:"column:original_price" json:"original_price,omitempty"`
	Currency      string        `gorm:"column:currency;size:3;default:KRW" json:"currency"`
	Condition     ItemCondition `gorm:"column:condition;size:20;default:good" json:"condition"`
	Status        ItemStatus    `gorm:"column:status;size:20;default:selling;index" json:"status"`
	TradeMethod   TradeMethod   `gorm:"column:trade_method;size:20;default:both" json:"trade_method"`
	Location      string        `gorm:"column:location;size:100" json:"location"`
	IsNegotiable  bool          `gorm:"column:is_negotiable;default:true" json:"is_negotiable"`
	ViewCount     uint          `gorm:"column:view_count;default:0" json:"view_count"`
	WishCount     uint          `gorm:"column:wish_count;default:0" json:"wish_count"`
	ChatCount     uint          `gorm:"column:chat_count;default:0" json:"chat_count"`
	Images        string        `gorm:"column:images;type:json" json:"images"` // JSON array
	BuyerID       *uint64       `gorm:"column:buyer_id" json:"buyer_id,omitempty"`
	SoldAt        *time.Time    `gorm:"column:sold_at" json:"sold_at,omitempty"`
	BumpedAt      *time.Time    `gorm:"column:bumped_at" json:"bumped_at,omitempty"` // 끌올 시간
	CreatedAt     time.Time     `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time     `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`

	// Relations
	Category *Category `gorm:"foreignKey:CategoryID" json:"category,omitempty"`
}

func (Item) TableName() string {
	return "marketplace_items"
}

// CreateItemRequest 상품 등록 요청
type CreateItemRequest struct {
	CategoryID    *uint64       `json:"category_id"`
	Title         string        `json:"title" binding:"required,max=200"`
	Description   string        `json:"description" binding:"required"`
	Price         int64         `json:"price" binding:"required,gte=0"`
	OriginalPrice *int64        `json:"original_price"`
	Condition     ItemCondition `json:"condition" binding:"required"`
	TradeMethod   TradeMethod   `json:"trade_method" binding:"required"`
	Location      string        `json:"location" binding:"max=100"`
	IsNegotiable  bool          `json:"is_negotiable"`
	Images        []string      `json:"images" binding:"max=10"`
}

// UpdateItemRequest 상품 수정 요청
type UpdateItemRequest struct {
	CategoryID    *uint64        `json:"category_id"`
	Title         *string        `json:"title" binding:"omitempty,max=200"`
	Description   *string        `json:"description"`
	Price         *int64         `json:"price" binding:"omitempty,gte=0"`
	OriginalPrice *int64         `json:"original_price"`
	Condition     *ItemCondition `json:"condition"`
	TradeMethod   *TradeMethod   `json:"trade_method"`
	Location      *string        `json:"location" binding:"omitempty,max=100"`
	IsNegotiable  *bool          `json:"is_negotiable"`
	Images        []string       `json:"images" binding:"omitempty,max=10"`
}

// UpdateStatusRequest 상태 변경 요청
type UpdateStatusRequest struct {
	Status  ItemStatus `json:"status" binding:"required"`
	BuyerID *uint64    `json:"buyer_id"`
}

// ItemResponse 상품 응답
type ItemResponse struct {
	ID            uint64            `json:"id"`
	SellerID      uint64            `json:"seller_id"`
	SellerName    string            `json:"seller_name,omitempty"`
	Category      *CategoryResponse `json:"category,omitempty"`
	Title         string            `json:"title"`
	Description   string            `json:"description"`
	Price         int64             `json:"price"`
	OriginalPrice *int64            `json:"original_price,omitempty"`
	Currency      string            `json:"currency"`
	Condition     ItemCondition     `json:"condition"`
	ConditionText string            `json:"condition_text"`
	Status        ItemStatus        `json:"status"`
	StatusText    string            `json:"status_text"`
	TradeMethod   TradeMethod       `json:"trade_method"`
	TradeText     string            `json:"trade_text"`
	Location      string            `json:"location"`
	IsNegotiable  bool              `json:"is_negotiable"`
	ViewCount     uint              `json:"view_count"`
	WishCount     uint              `json:"wish_count"`
	ChatCount     uint              `json:"chat_count"`
	Images        []string          `json:"images"`
	IsWished      bool              `json:"is_wished"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
	TimeAgo       string            `json:"time_ago"`
}

// ItemListResponse 상품 목록 응답
type ItemListResponse struct {
	ID           uint64        `json:"id"`
	SellerID     uint64        `json:"seller_id"`
	SellerName   string        `json:"seller_name,omitempty"`
	CategoryName string        `json:"category_name,omitempty"`
	Title        string        `json:"title"`
	Price        int64         `json:"price"`
	Currency     string        `json:"currency"`
	Condition    ItemCondition `json:"condition"`
	Status       ItemStatus    `json:"status"`
	StatusText   string        `json:"status_text"`
	Location     string        `json:"location"`
	IsNegotiable bool          `json:"is_negotiable"`
	ViewCount    uint          `json:"view_count"`
	WishCount    uint          `json:"wish_count"`
	ChatCount    uint          `json:"chat_count"`
	Thumbnail    string        `json:"thumbnail"`
	IsWished     bool          `json:"is_wished"`
	CreatedAt    time.Time     `json:"created_at"`
	TimeAgo      string        `json:"time_ago"`
}

// GetConditionText 상태 텍스트 반환
func GetConditionText(c ItemCondition) string {
	switch c {
	case ConditionNew:
		return "새상품"
	case ConditionLikeNew:
		return "거의 새것"
	case ConditionGood:
		return "상태 좋음"
	case ConditionFair:
		return "사용감 있음"
	case ConditionPoor:
		return "상태 나쁨"
	default:
		return string(c)
	}
}

// GetStatusText 상태 텍스트 반환
func GetStatusText(s ItemStatus) string {
	switch s {
	case ItemStatusSelling:
		return "판매중"
	case ItemStatusReserved:
		return "예약중"
	case ItemStatusSold:
		return "판매완료"
	case ItemStatusHidden:
		return "숨김"
	default:
		return string(s)
	}
}

// GetTradeText 거래방법 텍스트 반환
func GetTradeText(t TradeMethod) string {
	switch t {
	case TradeMethodDirect:
		return "직거래"
	case TradeMethodDelivery:
		return "택배"
	case TradeMethodBoth:
		return "직거래/택배"
	default:
		return string(t)
	}
}
