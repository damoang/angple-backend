package domain

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// ReviewStatus 리뷰 상태
type ReviewStatus string

const (
	ReviewStatusPending  ReviewStatus = "pending"
	ReviewStatusApproved ReviewStatus = "approved"
	ReviewStatusRejected ReviewStatus = "rejected"
	ReviewStatusHidden   ReviewStatus = "hidden"
)

// Review 리뷰 엔티티
type Review struct {
	ID          uint64 `gorm:"primaryKey" json:"id"`
	ProductID   uint64 `gorm:"column:product_id;not null" json:"product_id"`
	UserID      uint64 `gorm:"column:user_id;not null" json:"user_id"`
	OrderItemID uint64 `gorm:"column:order_item_id;not null" json:"order_item_id"`

	// 리뷰 내용
	Rating  uint8  `gorm:"not null" json:"rating"`
	Title   string `gorm:"size:200" json:"title,omitempty"`
	Content string `gorm:"type:text;not null" json:"content"`

	// 이미지
	Images string `gorm:"type:json" json:"-"`

	// 상태
	Status     ReviewStatus `gorm:"size:20;default:'pending'" json:"status"`
	IsVerified bool         `gorm:"column:is_verified;default:true" json:"is_verified"`

	// 도움됨
	HelpfulCount uint `gorm:"column:helpful_count;default:0" json:"helpful_count"`

	// 판매자 답글
	SellerReply string     `gorm:"column:seller_reply;type:text" json:"seller_reply,omitempty"`
	RepliedAt   *time.Time `gorm:"column:replied_at" json:"replied_at,omitempty"`

	// 타임스탬프
	CreatedAt time.Time      `gorm:"column:created_at" json:"created_at"`
	UpdatedAt time.Time      `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index" json:"-"`

	// Relations
	Product   *Product   `gorm:"foreignKey:ProductID" json:"-"`
	OrderItem *OrderItem `gorm:"foreignKey:OrderItemID" json:"-"`
}

// TableName GORM 테이블명
func (Review) TableName() string {
	return "commerce_reviews"
}

// GetImages Images JSON을 파싱하여 문자열 슬라이스로 반환
func (r *Review) GetImages() []string {
	if r.Images == "" || r.Images == "null" {
		return nil
	}
	var images []string
	if err := json.Unmarshal([]byte(r.Images), &images); err != nil {
		return nil
	}
	return images
}

// SetImages 문자열 슬라이스를 Images JSON으로 설정
func (r *Review) SetImages(images []string) error {
	if len(images) == 0 {
		r.Images = ""
		return nil
	}
	data, err := json.Marshal(images)
	if err != nil {
		return err
	}
	r.Images = string(data)
	return nil
}

// ReviewHelpful 리뷰 도움됨
type ReviewHelpful struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	ReviewID  uint64    `gorm:"column:review_id;not null" json:"review_id"`
	UserID    uint64    `gorm:"column:user_id;not null" json:"user_id"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`

	// Relations
	Review *Review `gorm:"foreignKey:ReviewID" json:"-"`
}

// TableName GORM 테이블명
func (ReviewHelpful) TableName() string {
	return "commerce_review_helpfuls"
}

// CreateReviewRequest 리뷰 작성 요청 DTO
type CreateReviewRequest struct {
	ProductID   uint64   `json:"product_id" binding:"required"`
	OrderItemID uint64   `json:"order_item_id" binding:"required"`
	Rating      uint8    `json:"rating" binding:"required,gte=1,lte=5"`
	Title       string   `json:"title" binding:"omitempty,max=200"`
	Content     string   `json:"content" binding:"required,min=10,max=5000"`
	Images      []string `json:"images" binding:"omitempty,max=5"`
}

// UpdateReviewRequest 리뷰 수정 요청 DTO
type UpdateReviewRequest struct {
	Rating  *uint8   `json:"rating" binding:"omitempty,gte=1,lte=5"`
	Title   *string  `json:"title" binding:"omitempty,max=200"`
	Content *string  `json:"content" binding:"omitempty,min=10,max=5000"`
	Images  []string `json:"images" binding:"omitempty,max=5"`
}

// ReplyReviewRequest 판매자 답글 요청 DTO
type ReplyReviewRequest struct {
	Reply string `json:"reply" binding:"required,min=1,max=1000"`
}

// ReviewListRequest 리뷰 목록 조회 요청
type ReviewListRequest struct {
	Page      int    `form:"page" binding:"omitempty,gte=1"`
	Limit     int    `form:"limit" binding:"omitempty,gte=1,lte=100"`
	Rating    *uint8 `form:"rating" binding:"omitempty,gte=1,lte=5"`
	SortBy    string `form:"sort_by" binding:"omitempty,oneof=created_at rating helpful_count"`
	SortOrder string `form:"sort_order" binding:"omitempty,oneof=asc desc"`
}

// ReviewResponse 리뷰 응답 DTO
type ReviewResponse struct {
	ID           uint64     `json:"id"`
	ProductID    uint64     `json:"product_id"`
	UserID       uint64     `json:"user_id"`
	UserName     string     `json:"user_name,omitempty"`
	OrderItemID  uint64     `json:"order_item_id"`
	Rating       uint8      `json:"rating"`
	Title        string     `json:"title,omitempty"`
	Content      string     `json:"content"`
	Images       []string   `json:"images,omitempty"`
	Status       string     `json:"status"`
	IsVerified   bool       `json:"is_verified"`
	HelpfulCount uint       `json:"helpful_count"`
	SellerReply  string     `json:"seller_reply,omitempty"`
	RepliedAt    *time.Time `json:"replied_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// ReviewSummary 리뷰 요약 (상품별)
type ReviewSummary struct {
	ProductID    uint64             `json:"product_id"`
	TotalCount   int64              `json:"total_count"`
	AverageRating float64           `json:"average_rating"`
	RatingCounts map[uint8]int64    `json:"rating_counts"`
}

// ToResponse Review를 ReviewResponse로 변환
func (r *Review) ToResponse() *ReviewResponse {
	return &ReviewResponse{
		ID:           r.ID,
		ProductID:    r.ProductID,
		UserID:       r.UserID,
		OrderItemID:  r.OrderItemID,
		Rating:       r.Rating,
		Title:        r.Title,
		Content:      r.Content,
		Images:       r.GetImages(),
		Status:       string(r.Status),
		IsVerified:   r.IsVerified,
		HelpfulCount: r.HelpfulCount,
		SellerReply:  r.SellerReply,
		RepliedAt:    r.RepliedAt,
		CreatedAt:    r.CreatedAt,
		UpdatedAt:    r.UpdatedAt,
	}
}
