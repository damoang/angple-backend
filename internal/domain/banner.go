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
// Table: banners (실제 DB 스키마에 맞춤)
type Banner struct {
	ID           string         `gorm:"column:id;primaryKey" json:"id"`
	AdvertiserID string         `gorm:"column:advertiser_id" json:"advertiser_id,omitempty"`
	Name         string         `gorm:"column:name" json:"name"`
	ImageURL     string         `gorm:"column:image_url" json:"image_url"`
	LandingURL   string         `gorm:"column:landing_url" json:"landing_url"`
	Position     BannerPosition `gorm:"column:position" json:"position"`
	StartDate    *time.Time     `gorm:"column:start_date" json:"start_date"`
	EndDate      *time.Time     `gorm:"column:end_date" json:"end_date"`
	Status       string         `gorm:"column:status" json:"status"`
	RejectReason string         `gorm:"column:reject_reason" json:"reject_reason,omitempty"`
	AltText      string         `gorm:"column:alt_text" json:"alt_text,omitempty"`
	Target       string         `gorm:"column:target" json:"target"`
	Memo         string         `gorm:"column:memo" json:"memo,omitempty"`
	CreatedAt    time.Time      `gorm:"column:created_at" json:"created_at"`
	UpdatedAt    time.Time      `gorm:"column:updated_at" json:"updated_at"`

	// 레거시 호환 필드 (사용하지 않지만 기존 코드 호환용)
	Title      string `gorm:"-" json:"title,omitempty"`
	LinkURL    string `gorm:"-" json:"link_url,omitempty"`
	IsActive   bool   `gorm:"-" json:"is_active,omitempty"`
	Priority   int    `gorm:"-" json:"priority,omitempty"`
	ClickCount int    `gorm:"-" json:"click_count,omitempty"`
	ViewCount  int    `gorm:"-" json:"view_count,omitempty"`
}

// TableName specifies the table name for Banner model
func (Banner) TableName() string {
	return "banners"
}

// BannerClickLog represents a click log for a banner
// Table: banner_click_logs
type BannerClickLog struct {
	ID        int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	BannerID  string    `gorm:"column:banner_id" json:"banner_id"`
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
	ID           string         `json:"id"`
	AdvertiserID string         `json:"advertiser_id,omitempty"`
	Name         string         `json:"name"`
	ImageURL     string         `json:"image_url"`
	LandingURL   string         `json:"landing_url"`
	Position     BannerPosition `json:"position"`
	StartDate    *time.Time     `json:"start_date,omitempty"`
	EndDate      *time.Time     `json:"end_date,omitempty"`
	Status       string         `json:"status"`
	AltText      string         `json:"alt_text,omitempty"`
	Target       string         `json:"target"`
	Memo         string         `json:"memo,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
}

// ToResponse converts Banner to BannerResponse
func (b *Banner) ToResponse() BannerResponse {
	return BannerResponse{
		ID:           b.ID,
		AdvertiserID: b.AdvertiserID,
		Name:         b.Name,
		ImageURL:     b.ImageURL,
		LandingURL:   b.LandingURL,
		Position:     b.Position,
		StartDate:    b.StartDate,
		EndDate:      b.EndDate,
		Status:       b.Status,
		AltText:      b.AltText,
		Target:       b.Target,
		Memo:         b.Memo,
		CreatedAt:    b.CreatedAt,
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
	BannerID   string  `json:"banner_id"`
	Name       string  `json:"name"`
	ClickCount int     `json:"click_count"`
	ViewCount  int     `json:"view_count"`
	CTR        float64 `json:"ctr"` // Click-through rate
}
