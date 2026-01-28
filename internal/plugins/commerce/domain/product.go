package domain

import (
	"time"

	"gorm.io/gorm"
)

// ProductType 상품 유형
type ProductType string

const (
	ProductTypeDigital  ProductType = "digital"
	ProductTypePhysical ProductType = "physical"
)

// ProductStatus 상품 상태
type ProductStatus string

const (
	ProductStatusDraft     ProductStatus = "draft"
	ProductStatusPublished ProductStatus = "published"
	ProductStatusArchived  ProductStatus = "archived"
)

// StockStatus 재고 상태
type StockStatus string

const (
	StockStatusInStock    StockStatus = "in_stock"
	StockStatusOutOfStock StockStatus = "out_of_stock"
	StockStatusPreorder   StockStatus = "preorder"
)

// Product 상품 엔티티
type Product struct {
	ID             uint64         `gorm:"primaryKey" json:"id"`
	SellerID       uint64         `gorm:"not null" json:"seller_id"`
	Name           string         `gorm:"size:255;not null" json:"name"`
	Slug           string         `gorm:"size:255;uniqueIndex" json:"slug"`
	Description    string         `gorm:"type:text" json:"description"`
	ShortDesc      string         `gorm:"column:short_desc;size:500" json:"short_desc"`
	ProductType    ProductType    `gorm:"column:product_type;size:20;not null;default:'digital'" json:"product_type"`
	Price          float64        `gorm:"type:decimal(12,2);not null;default:0" json:"price"`
	OriginalPrice  *float64       `gorm:"column:original_price;type:decimal(12,2)" json:"original_price,omitempty"`
	Currency       string         `gorm:"size:3;default:'KRW'" json:"currency"`
	StockQuantity  *int           `gorm:"column:stock_quantity" json:"stock_quantity,omitempty"`
	StockStatus    StockStatus    `gorm:"column:stock_status;size:20;default:'in_stock'" json:"stock_status"`
	DownloadLimit  *int           `gorm:"column:download_limit" json:"download_limit,omitempty"`
	DownloadExpiry *int           `gorm:"column:download_expiry" json:"download_expiry,omitempty"`
	Status         ProductStatus  `gorm:"size:20;default:'draft'" json:"status"`
	Visibility     string         `gorm:"size:20;default:'public'" json:"visibility"`
	Password       string         `gorm:"size:100" json:"-"`
	FeaturedImage  string         `gorm:"column:featured_image;size:500" json:"featured_image,omitempty"`
	GalleryImages  string         `gorm:"column:gallery_images;type:json" json:"gallery_images,omitempty"`
	MetaData       string         `gorm:"column:meta_data;type:json" json:"meta_data,omitempty"`
	SalesCount     uint           `gorm:"column:sales_count;default:0" json:"sales_count"`
	ViewCount      uint           `gorm:"column:view_count;default:0" json:"view_count"`
	RatingAvg      float64        `gorm:"column:rating_avg;type:decimal(2,1);default:0" json:"rating_avg"`
	RatingCount    uint           `gorm:"column:rating_count;default:0" json:"rating_count"`
	PublishedAt    *time.Time     `gorm:"column:published_at" json:"published_at,omitempty"`
	CreatedAt      time.Time      `gorm:"column:created_at" json:"created_at"`
	UpdatedAt      time.Time      `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"column:deleted_at;index" json:"-"`
}

// TableName GORM 테이블명
func (Product) TableName() string {
	return "commerce_products"
}

// ProductResponse 상품 응답 DTO
type ProductResponse struct {
	ID             uint64     `json:"id"`
	SellerID       uint64     `json:"seller_id"`
	Name           string     `json:"name"`
	Slug           string     `json:"slug"`
	Description    string     `json:"description,omitempty"`
	ShortDesc      string     `json:"short_desc,omitempty"`
	ProductType    string     `json:"product_type"`
	Price          float64    `json:"price"`
	OriginalPrice  *float64   `json:"original_price,omitempty"`
	Currency       string     `json:"currency"`
	StockQuantity  *int       `json:"stock_quantity,omitempty"`
	StockStatus    string     `json:"stock_status"`
	DownloadLimit  *int       `json:"download_limit,omitempty"`
	DownloadExpiry *int       `json:"download_expiry,omitempty"`
	Status         string     `json:"status"`
	Visibility     string     `json:"visibility"`
	FeaturedImage  string     `json:"featured_image,omitempty"`
	SalesCount     uint       `json:"sales_count"`
	ViewCount      uint       `json:"view_count"`
	RatingAvg      float64    `json:"rating_avg"`
	RatingCount    uint       `json:"rating_count"`
	PublishedAt    *time.Time `json:"published_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// ToResponse Product를 ProductResponse로 변환
func (p *Product) ToResponse() *ProductResponse {
	return &ProductResponse{
		ID:             p.ID,
		SellerID:       p.SellerID,
		Name:           p.Name,
		Slug:           p.Slug,
		Description:    p.Description,
		ShortDesc:      p.ShortDesc,
		ProductType:    string(p.ProductType),
		Price:          p.Price,
		OriginalPrice:  p.OriginalPrice,
		Currency:       p.Currency,
		StockQuantity:  p.StockQuantity,
		StockStatus:    string(p.StockStatus),
		DownloadLimit:  p.DownloadLimit,
		DownloadExpiry: p.DownloadExpiry,
		Status:         string(p.Status),
		Visibility:     p.Visibility,
		FeaturedImage:  p.FeaturedImage,
		SalesCount:     p.SalesCount,
		ViewCount:      p.ViewCount,
		RatingAvg:      p.RatingAvg,
		RatingCount:    p.RatingCount,
		PublishedAt:    p.PublishedAt,
		CreatedAt:      p.CreatedAt,
		UpdatedAt:      p.UpdatedAt,
	}
}

// CreateProductRequest 상품 생성 요청 DTO
type CreateProductRequest struct {
	Name           string   `json:"name" binding:"required,min=1,max=255"`
	Slug           string   `json:"slug" binding:"omitempty,max=255"`
	Description    string   `json:"description" binding:"omitempty"`
	ShortDesc      string   `json:"short_desc" binding:"omitempty,max=500"`
	ProductType    string   `json:"product_type" binding:"required,oneof=digital physical"`
	Price          float64  `json:"price" binding:"required,gte=0"`
	OriginalPrice  *float64 `json:"original_price" binding:"omitempty,gte=0"`
	Currency       string   `json:"currency" binding:"omitempty,len=3"`
	StockQuantity  *int     `json:"stock_quantity" binding:"omitempty,gte=0"`
	DownloadLimit  *int     `json:"download_limit" binding:"omitempty,gte=0"`
	DownloadExpiry *int     `json:"download_expiry" binding:"omitempty,gte=0"`
	Status         string   `json:"status" binding:"omitempty,oneof=draft published archived"`
	Visibility     string   `json:"visibility" binding:"omitempty,oneof=public private password"`
	Password       string   `json:"password" binding:"omitempty,max=100"`
	FeaturedImage  string   `json:"featured_image" binding:"omitempty,max=500"`
}

// UpdateProductRequest 상품 수정 요청 DTO
type UpdateProductRequest struct {
	Name           *string  `json:"name" binding:"omitempty,min=1,max=255"`
	Slug           *string  `json:"slug" binding:"omitempty,max=255"`
	Description    *string  `json:"description" binding:"omitempty"`
	ShortDesc      *string  `json:"short_desc" binding:"omitempty,max=500"`
	ProductType    *string  `json:"product_type" binding:"omitempty,oneof=digital physical"`
	Price          *float64 `json:"price" binding:"omitempty,gte=0"`
	OriginalPrice  *float64 `json:"original_price" binding:"omitempty,gte=0"`
	Currency       *string  `json:"currency" binding:"omitempty,len=3"`
	StockQuantity  *int     `json:"stock_quantity" binding:"omitempty,gte=0"`
	StockStatus    *string  `json:"stock_status" binding:"omitempty,oneof=in_stock out_of_stock preorder"`
	DownloadLimit  *int     `json:"download_limit" binding:"omitempty,gte=0"`
	DownloadExpiry *int     `json:"download_expiry" binding:"omitempty,gte=0"`
	Status         *string  `json:"status" binding:"omitempty,oneof=draft published archived"`
	Visibility     *string  `json:"visibility" binding:"omitempty,oneof=public private password"`
	Password       *string  `json:"password" binding:"omitempty,max=100"`
	FeaturedImage  *string  `json:"featured_image" binding:"omitempty,max=500"`
}

// ProductListRequest 상품 목록 조회 요청
type ProductListRequest struct {
	Page        int    `form:"page" binding:"omitempty,gte=1"`
	Limit       int    `form:"limit" binding:"omitempty,gte=1,lte=100"`
	ProductType string `form:"product_type" binding:"omitempty,oneof=digital physical"`
	Status      string `form:"status" binding:"omitempty,oneof=draft published archived"`
	Search      string `form:"search" binding:"omitempty,max=100"`
	SortBy      string `form:"sort_by" binding:"omitempty,oneof=created_at updated_at price sales_count view_count"`
	SortOrder   string `form:"sort_order" binding:"omitempty,oneof=asc desc"`
}
