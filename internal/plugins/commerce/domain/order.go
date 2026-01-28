package domain

import (
	"time"

	"gorm.io/gorm"
)

// OrderStatus 주문 상태
type OrderStatus string

const (
	OrderStatusPending    OrderStatus = "pending"    // 결제 대기
	OrderStatusPaid       OrderStatus = "paid"       // 결제 완료
	OrderStatusProcessing OrderStatus = "processing" // 처리 중 (디지털: 다운로드 준비, 실물: 배송 준비)
	OrderStatusShipped    OrderStatus = "shipped"    // 배송 중 (실물 상품)
	OrderStatusDelivered  OrderStatus = "delivered"  // 배송 완료 (실물 상품)
	OrderStatusCompleted  OrderStatus = "completed"  // 주문 완료
	OrderStatusCancelled  OrderStatus = "cancelled"  // 취소됨
	OrderStatusRefunded   OrderStatus = "refunded"   // 환불됨
)

// OrderItemStatus 주문 아이템 상태
type OrderItemStatus string

const (
	OrderItemStatusPending    OrderItemStatus = "pending"
	OrderItemStatusProcessing OrderItemStatus = "processing"
	OrderItemStatusCompleted  OrderItemStatus = "completed"
	OrderItemStatusRefunded   OrderItemStatus = "refunded"
)

// Order 주문 엔티티
type Order struct {
	ID          uint64      `gorm:"primaryKey" json:"id"`
	OrderNumber string      `gorm:"column:order_number;size:32;uniqueIndex;not null" json:"order_number"`
	UserID      uint64      `gorm:"column:user_id;not null" json:"user_id"`
	Subtotal    float64     `gorm:"type:decimal(12,2);not null" json:"subtotal"`
	Discount    float64     `gorm:"type:decimal(12,2);default:0" json:"discount"`
	ShippingFee float64     `gorm:"column:shipping_fee;type:decimal(12,2);default:0" json:"shipping_fee"`
	Total       float64     `gorm:"type:decimal(12,2);not null" json:"total"`
	Currency    string      `gorm:"size:3;default:'KRW'" json:"currency"`
	Status      OrderStatus `gorm:"size:20;default:'pending'" json:"status"`

	// 배송 정보 (실물 상품용)
	ShippingName    string `gorm:"column:shipping_name;size:100" json:"shipping_name,omitempty"`
	ShippingPhone   string `gorm:"column:shipping_phone;size:20" json:"shipping_phone,omitempty"`
	ShippingAddress string `gorm:"column:shipping_address;size:500" json:"shipping_address,omitempty"`
	ShippingPostal  string `gorm:"column:shipping_postal;size:10" json:"shipping_postal,omitempty"`
	ShippingMemo    string `gorm:"column:shipping_memo;size:255" json:"shipping_memo,omitempty"`

	// 송장 정보
	ShippingCarrier string     `gorm:"column:shipping_carrier;size:50" json:"shipping_carrier,omitempty"`
	TrackingNumber  string     `gorm:"column:tracking_number;size:100" json:"tracking_number,omitempty"`
	ShippedAt       *time.Time `gorm:"column:shipped_at" json:"shipped_at,omitempty"`
	DeliveredAt     *time.Time `gorm:"column:delivered_at" json:"delivered_at,omitempty"`

	// 메타
	IPAddress string `gorm:"column:ip_address;size:45" json:"-"`
	UserAgent string `gorm:"column:user_agent;size:500" json:"-"`
	MetaData  string `gorm:"column:meta_data;type:json" json:"meta_data,omitempty"`
	Notes     string `gorm:"type:text" json:"notes,omitempty"`

	// 타임스탬프
	PaidAt      *time.Time     `gorm:"column:paid_at" json:"paid_at,omitempty"`
	CompletedAt *time.Time     `gorm:"column:completed_at" json:"completed_at,omitempty"`
	CancelledAt *time.Time     `gorm:"column:cancelled_at" json:"cancelled_at,omitempty"`
	CreatedAt   time.Time      `gorm:"column:created_at" json:"created_at"`
	UpdatedAt   time.Time      `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"column:deleted_at;index" json:"-"`

	// Relations
	Items []OrderItem `gorm:"foreignKey:OrderID" json:"items,omitempty"`
}

// TableName GORM 테이블명
func (Order) TableName() string {
	return "commerce_orders"
}

// OrderItem 주문 아이템 엔티티
type OrderItem struct {
	ID        uint64 `gorm:"primaryKey" json:"id"`
	OrderID   uint64 `gorm:"column:order_id;not null" json:"order_id"`
	ProductID uint64 `gorm:"column:product_id;not null" json:"product_id"`
	SellerID  uint64 `gorm:"column:seller_id;not null" json:"seller_id"`

	// 상품 스냅샷 (주문 시점 정보 보존)
	ProductName string      `gorm:"column:product_name;size:255;not null" json:"product_name"`
	ProductType ProductType `gorm:"column:product_type;not null" json:"product_type"`

	// 금액
	Price    float64 `gorm:"type:decimal(12,2);not null" json:"price"`
	Quantity int     `gorm:"not null;default:1" json:"quantity"`
	Subtotal float64 `gorm:"type:decimal(12,2);not null" json:"subtotal"`

	// 정산 정보
	PlatformFeeRate *float64 `gorm:"column:platform_fee_rate;type:decimal(5,2)" json:"platform_fee_rate,omitempty"`
	PlatformFee     *float64 `gorm:"column:platform_fee;type:decimal(12,2)" json:"platform_fee,omitempty"`
	SellerAmount    *float64 `gorm:"column:seller_amount;type:decimal(12,2)" json:"seller_amount,omitempty"`

	// 상태
	Status           OrderItemStatus `gorm:"size:20;default:'pending'" json:"status"`
	SettlementStatus string          `gorm:"column:settlement_status;size:20;default:'pending'" json:"settlement_status,omitempty"`

	// 메타
	MetaData string `gorm:"column:meta_data;type:json" json:"meta_data,omitempty"`

	// 타임스탬프
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at" json:"updated_at"`

	// Relations
	Order   *Order   `gorm:"foreignKey:OrderID" json:"-"`
	Product *Product `gorm:"foreignKey:ProductID" json:"-"`
}

// TableName GORM 테이블명
func (OrderItem) TableName() string {
	return "commerce_order_items"
}

// CreateOrderRequest 주문 생성 요청 DTO
type CreateOrderRequest struct {
	// 배송 정보 (실물 상품 포함 시 필수)
	ShippingName    string `json:"shipping_name" binding:"omitempty,max=100"`
	ShippingPhone   string `json:"shipping_phone" binding:"omitempty,max=20"`
	ShippingAddress string `json:"shipping_address" binding:"omitempty,max=500"`
	ShippingPostal  string `json:"shipping_postal" binding:"omitempty,max=10"`
	ShippingMemo    string `json:"shipping_memo" binding:"omitempty,max=255"`
}

// OrderListRequest 주문 목록 조회 요청
type OrderListRequest struct {
	Page      int    `form:"page" binding:"omitempty,gte=1"`
	Limit     int    `form:"limit" binding:"omitempty,gte=1,lte=100"`
	Status    string `form:"status" binding:"omitempty,oneof=pending paid processing shipped delivered completed cancelled refunded"`
	SortBy    string `form:"sort_by" binding:"omitempty,oneof=created_at total"`
	SortOrder string `form:"sort_order" binding:"omitempty,oneof=asc desc"`
}

// CancelOrderRequest 주문 취소 요청 DTO
type CancelOrderRequest struct {
	Reason string `json:"reason" binding:"omitempty,max=255"`
}

// OrderItemResponse 주문 아이템 응답 DTO
type OrderItemResponse struct {
	ID          uint64  `json:"id"`
	ProductID   uint64  `json:"product_id"`
	ProductName string  `json:"product_name"`
	ProductType string  `json:"product_type"`
	Price       float64 `json:"price"`
	Quantity    int     `json:"quantity"`
	Subtotal    float64 `json:"subtotal"`
	Status      string  `json:"status"`
}

// OrderResponse 주문 응답 DTO
type OrderResponse struct {
	ID          uint64               `json:"id"`
	OrderNumber string               `json:"order_number"`
	Subtotal    float64              `json:"subtotal"`
	Discount    float64              `json:"discount"`
	ShippingFee float64              `json:"shipping_fee"`
	Total       float64              `json:"total"`
	Currency    string               `json:"currency"`
	Status      string               `json:"status"`
	Items       []*OrderItemResponse `json:"items"`

	// 배송 정보 (실물 상품 포함 시)
	ShippingName    string `json:"shipping_name,omitempty"`
	ShippingPhone   string `json:"shipping_phone,omitempty"`
	ShippingAddress string `json:"shipping_address,omitempty"`
	ShippingPostal  string `json:"shipping_postal,omitempty"`
	ShippingMemo    string `json:"shipping_memo,omitempty"`

	// 송장 정보
	ShippingCarrier string     `json:"shipping_carrier,omitempty"`
	TrackingNumber  string     `json:"tracking_number,omitempty"`
	ShippedAt       *time.Time `json:"shipped_at,omitempty"`
	DeliveredAt     *time.Time `json:"delivered_at,omitempty"`

	// 타임스탬프
	PaidAt      *time.Time `json:"paid_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CancelledAt *time.Time `json:"cancelled_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// ToResponse Order를 OrderResponse로 변환
func (o *Order) ToResponse() *OrderResponse {
	response := &OrderResponse{
		ID:              o.ID,
		OrderNumber:     o.OrderNumber,
		Subtotal:        o.Subtotal,
		Discount:        o.Discount,
		ShippingFee:     o.ShippingFee,
		Total:           o.Total,
		Currency:        o.Currency,
		Status:          string(o.Status),
		ShippingName:    o.ShippingName,
		ShippingPhone:   o.ShippingPhone,
		ShippingAddress: o.ShippingAddress,
		ShippingPostal:  o.ShippingPostal,
		ShippingMemo:    o.ShippingMemo,
		ShippingCarrier: o.ShippingCarrier,
		TrackingNumber:  o.TrackingNumber,
		ShippedAt:       o.ShippedAt,
		DeliveredAt:     o.DeliveredAt,
		PaidAt:          o.PaidAt,
		CompletedAt:     o.CompletedAt,
		CancelledAt:     o.CancelledAt,
		CreatedAt:       o.CreatedAt,
		Items:           make([]*OrderItemResponse, 0, len(o.Items)),
	}

	for _, item := range o.Items {
		response.Items = append(response.Items, item.ToResponse())
	}

	return response
}

// ToResponse OrderItem을 OrderItemResponse로 변환
func (oi *OrderItem) ToResponse() *OrderItemResponse {
	return &OrderItemResponse{
		ID:          oi.ID,
		ProductID:   oi.ProductID,
		ProductName: oi.ProductName,
		ProductType: string(oi.ProductType),
		Price:       oi.Price,
		Quantity:    oi.Quantity,
		Subtotal:    oi.Subtotal,
		Status:      string(oi.Status),
	}
}

// HasPhysicalProduct 주문에 실물 상품이 포함되어 있는지 확인
func (o *Order) HasPhysicalProduct() bool {
	for _, item := range o.Items {
		if item.ProductType == ProductTypePhysical {
			return true
		}
	}
	return false
}

// HasDigitalProduct 주문에 디지털 상품이 포함되어 있는지 확인
func (o *Order) HasDigitalProduct() bool {
	for _, item := range o.Items {
		if item.ProductType == ProductTypeDigital {
			return true
		}
	}
	return false
}
