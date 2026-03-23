package gnuboard

import "time"

// AffiliateLink stores persisted mb-independent affiliate conversion results.
type AffiliateLink struct {
	ID              uint64     `gorm:"column:id;primaryKey;autoIncrement"`
	BoardSlug       string     `gorm:"column:board_slug;size:64;not null"`
	PostID          int        `gorm:"column:post_id;not null;default:0"`
	CommentID       int        `gorm:"column:comment_id;not null;default:0"`
	EntityType      string     `gorm:"column:entity_type;size:20;not null"`
	LinkIndex       int        `gorm:"column:link_index;not null;default:0"`
	SourceURL       string     `gorm:"column:source_url;type:text;not null"`
	NormalizedURL   string     `gorm:"column:normalized_url;type:text;not null"`
	MerchantDomain  string     `gorm:"column:merchant_domain;size:255;not null;default:''"`
	Platform        string     `gorm:"column:platform;size:50;not null;default:''"`
	AffiliateURL    string     `gorm:"column:affiliate_url;type:text"`
	RedirectID      string     `gorm:"column:redirect_id;size:32;not null;default:''"`
	Status          string     `gorm:"column:status;size:20;not null;default:'pending'"`
	ReasonCode      string     `gorm:"column:reason_code;size:50;not null;default:''"`
	AttemptCount    int        `gorm:"column:attempt_count;not null;default:0"`
	LastError       string     `gorm:"column:last_error;type:text"`
	LastProcessedAt *time.Time `gorm:"column:last_processed_at"`
	CreatedAt       time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt       time.Time  `gorm:"column:updated_at;autoUpdateTime"`
}

func (AffiliateLink) TableName() string {
	return "g5_affiliate_links"
}
