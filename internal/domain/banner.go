package domain

import (
	"time"
)

// BannerPosition represents the position of a banner
type BannerPosition string

const (
	BannerPositionHeader  BannerPosition = "header"
	BannerPositionSidebar BannerPosition = "sidebar"
	BannerPositionContent BannerPosition = "content"
	BannerPositionFooter  BannerPosition = "footer"
)

// Banner represents a banner in the system
// Table: banners
type Banner struct {
	ID         int64          `gorm:"column:id;primaryKey" json:"id"`
	Title      string         `gorm:"column:title" json:"title"`
	ImageURL   string         `gorm:"column:image_url" json:"image_url"`
	LinkURL    string         `gorm:"column:link_url" json:"link_url"`
	Position   BannerPosition `gorm:"column:position" json:"position"`
	StartDate  *time.Time     `gorm:"column:start_date" json:"start_date"`
	EndDate    *time.Time     `gorm:"column:end_date" json:"end_date"`
	Priority   int            `gorm:"column:priority" json:"priority"`
	IsActive   bool           `gorm:"column:is_active" json:"is_active"`
	ClickCount int            `gorm:"column:click_count" json:"click_count"`
	ViewCount  int            `gorm:"column:view_count" json:"view_count"`
	AltText    string         `gorm:"column:alt_text" json:"alt_text,omitempty"`
	Target     string         `gorm:"column:target" json:"target"`
	Memo       string         `gorm:"column:memo" json:"memo,omitempty"`
	CreatedAt  time.Time      `gorm:"column:created_at" json:"created_at"`
	UpdatedAt  time.Time      `gorm:"column:updated_at" json:"updated_at"`
}

// TableName specifies the table name for Banner model
func (Banner) TableName() string {
	return "banners"
}

// BannerClickLog represents a click log for a banner
// Table: banner_click_logs
type BannerClickLog struct {
	ID        int64     `gorm:"column:id;primaryKey" json:"id"`
	BannerID  int64     `gorm:"column:banner_id" json:"banner_id"`
	MemberID  string    `gorm:"column:member_id" json:"member_id,omitempty"`
	IPAddress string    `gorm:"column:ip_address" json:"ip_address,omitempty"`
	UserAgent string    `gorm:"column:user_agent" json:"user_agent,omitempty"`
	Referer   string    `gorm:"column:referer" json:"referer,omitempty"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
}

// TableName specifies the table name for BannerClickLog model
func (BannerClickLog) TableName() string {
	return "banner_click_logs"
}

// BannerResponse is the API response format for banner
type BannerResponse struct {
	ID         int64          `json:"id"`
	Title      string         `json:"title"`
	ImageURL   string         `json:"image_url"`
	LinkURL    string         `json:"link_url"`
	Position   BannerPosition `json:"position"`
	StartDate  *time.Time     `json:"start_date,omitempty"`
	EndDate    *time.Time     `json:"end_date,omitempty"`
	Priority   int            `json:"priority"`
	IsActive   bool           `json:"is_active"`
	ClickCount int            `json:"click_count"`
	ViewCount  int            `json:"view_count"`
	AltText    string         `json:"alt_text,omitempty"`
	Target     string         `json:"target"`
	Memo       string         `json:"memo,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
}

// ToResponse converts Banner to BannerResponse
func (b *Banner) ToResponse() BannerResponse {
	return BannerResponse{
		ID:         b.ID,
		Title:      b.Title,
		ImageURL:   b.ImageURL,
		LinkURL:    b.LinkURL,
		Position:   b.Position,
		StartDate:  b.StartDate,
		EndDate:    b.EndDate,
		Priority:   b.Priority,
		IsActive:   b.IsActive,
		ClickCount: b.ClickCount,
		ViewCount:  b.ViewCount,
		AltText:    b.AltText,
		Target:     b.Target,
		Memo:       b.Memo,
		CreatedAt:  b.CreatedAt,
	}
}

// BannerListResponse is the response for list of banners by position
type BannerListResponse struct {
	Banners  []BannerResponse `json:"banners"`
	Total    int              `json:"total"`
	Position BannerPosition   `json:"position,omitempty"`
}

// CreateBannerRequest is the request body for creating a banner
type CreateBannerRequest struct {
	Title     string         `json:"title" binding:"required"`
	ImageURL  string         `json:"image_url"`
	LinkURL   string         `json:"link_url"`
	Position  BannerPosition `json:"position" binding:"required"`
	StartDate *time.Time     `json:"start_date"`
	EndDate   *time.Time     `json:"end_date"`
	Priority  int            `json:"priority"`
	IsActive  bool           `json:"is_active"`
	AltText   string         `json:"alt_text"`
	Target    string         `json:"target"`
	Memo      string         `json:"memo"`
}

// UpdateBannerRequest is the request body for updating a banner
type UpdateBannerRequest struct {
	Title     string         `json:"title"`
	ImageURL  string         `json:"image_url"`
	LinkURL   string         `json:"link_url"`
	Position  BannerPosition `json:"position"`
	StartDate *time.Time     `json:"start_date"`
	EndDate   *time.Time     `json:"end_date"`
	Priority  int            `json:"priority"`
	IsActive  bool           `json:"is_active"`
	AltText   string         `json:"alt_text"`
	Target    string         `json:"target"`
	Memo      string         `json:"memo"`
}

// BannerClickRequest is the request body for tracking a banner click
type BannerClickRequest struct {
	MemberID  string `json:"member_id"`
	IPAddress string `json:"ip_address"`
	UserAgent string `json:"user_agent"`
	Referer   string `json:"referer"`
}

// BannerStatsResponse is the response for banner statistics
type BannerStatsResponse struct {
	BannerID   int64   `json:"banner_id"`
	Title      string  `json:"title"`
	ClickCount int     `json:"click_count"`
	ViewCount  int     `json:"view_count"`
	CTR        float64 `json:"ctr"` // Click-through rate
}
