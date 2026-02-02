package domain

import "time"

// PluginDeveloper 플러그인 개발자
type PluginDeveloper struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID      uint64    `gorm:"uniqueIndex;not null" json:"user_id"`
	DisplayName string    `gorm:"size:100;not null" json:"display_name"`
	Email       string    `gorm:"size:255;not null" json:"email"`
	Website     string    `gorm:"size:255" json:"website"`
	Bio         string    `gorm:"type:text" json:"bio"`
	IsVerified  bool      `gorm:"default:false" json:"is_verified"`
	Status      string    `gorm:"size:20;default:active" json:"status"` // active, suspended
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (PluginDeveloper) TableName() string { return "plugin_developers" }

// PluginSubmission 플러그인 제출 (마켓플레이스 등록 요청)
type PluginSubmission struct {
	ID            uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	DeveloperID   uint64     `gorm:"index;not null" json:"developer_id"`
	PluginName    string     `gorm:"size:100;not null" json:"plugin_name"`
	Version       string     `gorm:"size:20;not null" json:"version"`
	Title         string     `gorm:"size:200;not null" json:"title"`
	Description   string     `gorm:"type:text" json:"description"`
	Category      string     `gorm:"size:50" json:"category"`
	Tags          string     `gorm:"type:text" json:"tags"` // JSON array
	SourceURL     string     `gorm:"size:500" json:"source_url"`
	DownloadURL   string     `gorm:"size:500" json:"download_url"`
	Readme        string     `gorm:"type:longtext" json:"readme"`
	Status        string     `gorm:"size:20;default:pending;index" json:"status"` // pending, approved, rejected
	ReviewNote    string     `gorm:"type:text" json:"review_note,omitempty"`
	ReviewedBy    *uint64    `json:"reviewed_by,omitempty"`
	ReviewedAt    *time.Time `json:"reviewed_at,omitempty"`
	DownloadCount int64      `gorm:"default:0" json:"download_count"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

func (PluginSubmission) TableName() string { return "plugin_submissions" }

// PluginReview 플러그인 리뷰/평점
type PluginReview struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	PluginName string    `gorm:"size:100;index;not null" json:"plugin_name"`
	UserID     uint64    `gorm:"not null" json:"user_id"`
	Rating     int       `gorm:"not null" json:"rating"` // 1-5
	Comment    string    `gorm:"type:text" json:"comment"`
	CreatedAt  time.Time `json:"created_at"`
}

func (PluginReview) TableName() string { return "plugin_reviews" }

// PluginDownload 다운로드 추적
type PluginDownload struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	PluginName string    `gorm:"size:100;index;not null" json:"plugin_name"`
	Version    string    `gorm:"size:20" json:"version"`
	UserID     *uint64   `json:"user_id,omitempty"`
	IP         string    `gorm:"size:45" json:"ip"`
	CreatedAt  time.Time `json:"created_at"`
}

func (PluginDownload) TableName() string { return "plugin_downloads" }

// === Request/Response DTOs ===

// DeveloperRegisterRequest 개발자 등록 요청
type DeveloperRegisterRequest struct {
	DisplayName string `json:"display_name" binding:"required"`
	Email       string `json:"email" binding:"required,email"`
	Website     string `json:"website"`
	Bio         string `json:"bio"`
}

// PluginSubmitRequest 플러그인 제출 요청
type PluginSubmitRequest struct {
	PluginName  string `json:"plugin_name" binding:"required"`
	Version     string `json:"version" binding:"required"`
	Title       string `json:"title" binding:"required"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Tags        string `json:"tags"`
	SourceURL   string `json:"source_url"`
	DownloadURL string `json:"download_url" binding:"required"`
	Readme      string `json:"readme"`
}

// PluginReviewRequest 리뷰 작성 요청
type PluginReviewRequest struct {
	Rating  int    `json:"rating" binding:"required,min=1,max=5"`
	Comment string `json:"comment"`
}

// MarketplaceListItem 마켓플레이스 목록 아이템
type MarketplaceListItem struct {
	PluginName    string  `json:"plugin_name"`
	Version       string  `json:"version"`
	Title         string  `json:"title"`
	Description   string  `json:"description"`
	Category      string  `json:"category"`
	DeveloperName string  `json:"developer_name"`
	DownloadCount int64   `json:"download_count"`
	AvgRating     float64 `json:"avg_rating"`
	ReviewCount   int64   `json:"review_count"`
}
