package domain

import (
	"time"
)

// Cart 장바구니 엔티티
type Cart struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	UserID    uint64    `gorm:"column:user_id;not null" json:"user_id"`
	ProductID uint64    `gorm:"column:product_id;not null" json:"product_id"`
	Quantity  int       `gorm:"not null;default:1" json:"quantity"`
	MetaData  string    `gorm:"column:meta_data;type:json" json:"meta_data,omitempty"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at" json:"updated_at"`

	// Relations (조회 시 Preload 사용)
	Product *Product `gorm:"foreignKey:ProductID" json:"product,omitempty"`
}

// TableName GORM 테이블명
func (Cart) TableName() string {
	return "commerce_carts"
}

// AddToCartRequest 장바구니 추가 요청 DTO
type AddToCartRequest struct {
	ProductID uint64 `json:"product_id" binding:"required"`
	Quantity  int    `json:"quantity" binding:"required,gte=1"`
}

// UpdateCartRequest 장바구니 수량 변경 요청 DTO
type UpdateCartRequest struct {
	Quantity int `json:"quantity" binding:"required,gte=1"`
}

// CartItemResponse 장바구니 아이템 응답 DTO
type CartItemResponse struct {
	ID       uint64           `json:"id"`
	Product  *ProductResponse `json:"product"`
	Quantity int              `json:"quantity"`
	Subtotal float64          `json:"subtotal"`
}

// CartResponse 장바구니 전체 응답 DTO
type CartResponse struct {
	Items      []*CartItemResponse `json:"items"`
	ItemCount  int                 `json:"item_count"`
	TotalCount int                 `json:"total_count"` // 총 수량 (각 아이템의 quantity 합)
	Subtotal   float64             `json:"subtotal"`
	Currency   string              `json:"currency"`
}

// ToCartItemResponse Cart를 CartItemResponse로 변환
func (c *Cart) ToCartItemResponse() *CartItemResponse {
	response := &CartItemResponse{
		ID:       c.ID,
		Quantity: c.Quantity,
		Subtotal: 0,
	}

	if c.Product != nil {
		response.Product = c.Product.ToResponse()
		response.Subtotal = c.Product.Price * float64(c.Quantity)
	}

	return response
}
