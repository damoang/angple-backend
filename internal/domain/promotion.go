package domain

import (
	"time"
)

// Advertiser represents an advertiser in the promotion system
// Table: advertisers
type Advertiser struct {
	ID        int64      `gorm:"column:id;primaryKey" json:"id"`
	MemberID  string     `gorm:"column:member_id" json:"member_id"`
	Name      string     `gorm:"column:name" json:"name"`
	PostCount int        `gorm:"column:post_count" json:"post_count"`
	StartDate *time.Time `gorm:"column:start_date" json:"start_date"`
	EndDate   *time.Time `gorm:"column:end_date" json:"end_date"`
	IsPinned  bool       `gorm:"column:is_pinned" json:"is_pinned"`
	IsActive  bool       `gorm:"column:is_active" json:"is_active"`
	Memo      string     `gorm:"column:memo" json:"memo,omitempty"`
	CreatedAt time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt time.Time  `gorm:"column:updated_at" json:"updated_at"`
}

// TableName specifies the table name for Advertiser model
func (Advertiser) TableName() string {
	return "advertisers"
}

// PromotionPost represents a promotion post
// Table: promotion_posts
type PromotionPost struct {
	ID           int64     `gorm:"column:id;primaryKey" json:"id"`
	AdvertiserID int64     `gorm:"column:advertiser_id" json:"advertiser_id"`
	Title        string    `gorm:"column:title" json:"title"`
	Content      string    `gorm:"column:content" json:"content"`
	LinkURL      string    `gorm:"column:link_url" json:"link_url,omitempty"`
	ImageURL     string    `gorm:"column:image_url" json:"image_url,omitempty"`
	Views        int       `gorm:"column:views" json:"views"`
	Likes        int       `gorm:"column:likes" json:"likes"`
	CommentCount int       `gorm:"column:comment_count" json:"comment_count"`
	IsActive     bool      `gorm:"column:is_active" json:"is_active"`
	CreatedAt    time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at" json:"updated_at"`

	// Joined fields (not in DB)
	AuthorName string `gorm:"-" json:"author_name,omitempty"`
	IsPinned   bool   `gorm:"-" json:"is_pinned,omitempty"`
}

// TableName specifies the table name for PromotionPost model
func (PromotionPost) TableName() string {
	return "promotion_posts"
}

// AdvertiserResponse is the API response format for advertiser
type AdvertiserResponse struct {
	ID        int64      `json:"id"`
	MemberID  string     `json:"member_id"`
	Name      string     `json:"name"`
	PostCount int        `json:"post_count"`
	StartDate *time.Time `json:"start_date,omitempty"`
	EndDate   *time.Time `json:"end_date,omitempty"`
	IsPinned  bool       `json:"is_pinned"`
	IsActive  bool       `json:"is_active"`
	Memo      string     `json:"memo,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// ToResponse converts Advertiser to AdvertiserResponse
func (a *Advertiser) ToResponse() AdvertiserResponse {
	return AdvertiserResponse{
		ID:        a.ID,
		MemberID:  a.MemberID,
		Name:      a.Name,
		PostCount: a.PostCount,
		StartDate: a.StartDate,
		EndDate:   a.EndDate,
		IsPinned:  a.IsPinned,
		IsActive:  a.IsActive,
		Memo:      a.Memo,
		CreatedAt: a.CreatedAt,
	}
}

// PromotionPostResponse is the API response format for promotion post
type PromotionPostResponse struct {
	ID           int64     `json:"id"`
	AdvertiserID int64     `json:"advertiser_id"`
	Title        string    `json:"title"`
	Content      string    `json:"content"`
	LinkURL      string    `json:"link_url,omitempty"`
	ImageURL     string    `json:"image_url,omitempty"`
	Views        int       `json:"views"`
	Likes        int       `json:"likes"`
	CommentCount int       `json:"comment_count"`
	AuthorName   string    `json:"author_name,omitempty"`
	IsPinned     bool      `json:"is_pinned"`
	IsPromotion  bool      `json:"is_promotion"`
	CreatedAt    time.Time `json:"created_at"`
}

// ToResponse converts PromotionPost to PromotionPostResponse
func (p *PromotionPost) ToResponse() PromotionPostResponse {
	return PromotionPostResponse{
		ID:           p.ID,
		AdvertiserID: p.AdvertiserID,
		Title:        p.Title,
		Content:      p.Content,
		LinkURL:      p.LinkURL,
		ImageURL:     p.ImageURL,
		Views:        p.Views,
		Likes:        p.Likes,
		CommentCount: p.CommentCount,
		AuthorName:   p.AuthorName,
		IsPinned:     p.IsPinned,
		IsPromotion:  true,
		CreatedAt:    p.CreatedAt,
	}
}

// PromotionListResponse is the response for list of promotion posts
type PromotionListResponse struct {
	Posts       []PromotionPostResponse `json:"posts"`
	Total       int                     `json:"total"`
	Advertisers []AdvertiserResponse    `json:"advertisers,omitempty"`
}

// CreateAdvertiserRequest is the request body for creating an advertiser
type CreateAdvertiserRequest struct {
	MemberID  string     `json:"member_id" binding:"required"`
	Name      string     `json:"name" binding:"required"`
	PostCount int        `json:"post_count"`
	StartDate *time.Time `json:"start_date"`
	EndDate   *time.Time `json:"end_date"`
	IsPinned  bool       `json:"is_pinned"`
	IsActive  bool       `json:"is_active"`
	Memo      string     `json:"memo"`
}

// UpdateAdvertiserRequest is the request body for updating an advertiser
type UpdateAdvertiserRequest struct {
	Name      string     `json:"name"`
	PostCount int        `json:"post_count"`
	StartDate *time.Time `json:"start_date"`
	EndDate   *time.Time `json:"end_date"`
	IsPinned  bool       `json:"is_pinned"`
	IsActive  bool       `json:"is_active"`
	Memo      string     `json:"memo"`
}

// CreatePromotionPostRequest is the request body for creating a promotion post
type CreatePromotionPostRequest struct {
	Title    string `json:"title" binding:"required"`
	Content  string `json:"content"`
	LinkURL  string `json:"link_url"`
	ImageURL string `json:"image_url"`
}

// UpdatePromotionPostRequest is the request body for updating a promotion post
type UpdatePromotionPostRequest struct {
	Title    string `json:"title"`
	Content  string `json:"content"`
	LinkURL  string `json:"link_url"`
	ImageURL string `json:"image_url"`
	IsActive bool   `json:"is_active"`
}
