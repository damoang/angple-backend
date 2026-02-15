package domain

import (
	"time"
)

// Download 다운로드 로그 엔티티
type Download struct {
	ID             uint64     `gorm:"primaryKey" json:"id"`
	OrderItemID    uint64     `gorm:"column:order_item_id;not null" json:"order_item_id"`
	FileID         uint64     `gorm:"column:file_id;not null" json:"file_id"`
	UserID         uint64     `gorm:"column:user_id;not null" json:"user_id"`
	DownloadToken  string     `gorm:"column:download_token;size:64;uniqueIndex;not null" json:"download_token"`
	DownloadCount  int        `gorm:"column:download_count;default:0" json:"download_count"`
	LastDownloadAt *time.Time `gorm:"column:last_download_at" json:"last_download_at,omitempty"`
	ExpiresAt      *time.Time `gorm:"column:expires_at" json:"expires_at,omitempty"`
	IPAddress      string     `gorm:"column:ip_address;size:45" json:"-"`
	UserAgent      string     `gorm:"column:user_agent;size:500" json:"-"`
	CreatedAt      time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt      time.Time  `gorm:"column:updated_at" json:"updated_at"`

	// Relations
	OrderItem   *OrderItem   `gorm:"foreignKey:OrderItemID" json:"-"`
	ProductFile *ProductFile `gorm:"foreignKey:FileID" json:"-"`
}

// TableName GORM 테이블명
func (Download) TableName() string {
	return "commerce_downloads"
}

// IsExpired 다운로드 권한 만료 여부
func (d *Download) IsExpired() bool {
	if d.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*d.ExpiresAt)
}

// IsLimitReached 다운로드 횟수 제한 도달 여부
func (d *Download) IsLimitReached(limit int) bool {
	if limit <= 0 {
		return false
	}
	return d.DownloadCount >= limit
}

// DownloadResponse 다운로드 정보 응답 DTO
type DownloadResponse struct {
	ID             uint64               `json:"id"`
	OrderItemID    uint64               `json:"order_item_id"`
	File           *ProductFileResponse `json:"file"`
	DownloadToken  string               `json:"download_token"`
	DownloadCount  int                  `json:"download_count"`
	DownloadLimit  int                  `json:"download_limit,omitempty"`
	ExpiresAt      *time.Time           `json:"expires_at,omitempty"`
	LastDownloadAt *time.Time           `json:"last_download_at,omitempty"`
	IsExpired      bool                 `json:"is_expired"`
	CanDownload    bool                 `json:"can_download"`
}

// ToResponse Download를 DownloadResponse로 변환
func (d *Download) ToResponse(downloadLimit int) *DownloadResponse {
	response := &DownloadResponse{
		ID:             d.ID,
		OrderItemID:    d.OrderItemID,
		DownloadToken:  d.DownloadToken,
		DownloadCount:  d.DownloadCount,
		DownloadLimit:  downloadLimit,
		ExpiresAt:      d.ExpiresAt,
		LastDownloadAt: d.LastDownloadAt,
		IsExpired:      d.IsExpired(),
		CanDownload:    !d.IsExpired() && !d.IsLimitReached(downloadLimit),
	}

	if d.ProductFile != nil {
		response.File = d.ProductFile.ToResponse()
	}

	return response
}

// OrderDownloadsResponse 주문의 다운로드 목록 응답
type OrderDownloadsResponse struct {
	OrderID     uint64              `json:"order_id"`
	OrderNumber string              `json:"order_number"`
	Downloads   []*DownloadResponse `json:"downloads"`
}

// DownloadURLResponse 다운로드 URL 응답
type DownloadURLResponse struct {
	DownloadURL string    `json:"download_url"`
	ExpiresAt   time.Time `json:"expires_at"`
	FileName    string    `json:"file_name"`
	FileSize    int64     `json:"file_size"`
}
