//go:build ignore

// 이모티콘 플러그인 도메인 모델
package domain

import "time"

// EmoticonPack 이모티콘 팩 (카테고리)
type EmoticonPack struct {
	ID           int64     `gorm:"primaryKey" json:"id"`
	Slug         string    `gorm:"size:50;uniqueIndex;not null" json:"slug"`
	Name         string    `gorm:"size:100;not null" json:"name"`
	DefaultWidth int       `gorm:"default:50" json:"default_width"`
	IsActive     bool      `gorm:"default:true" json:"is_active"`
	SortOrder    int       `gorm:"default:0" json:"sort_order"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (EmoticonPack) TableName() string {
	return "emoticon_packs"
}

// EmoticonItem 개별 이모티콘
type EmoticonItem struct {
	ID        int64     `gorm:"primaryKey" json:"id"`
	PackID    int64     `gorm:"not null;index" json:"pack_id"`
	Filename  string    `gorm:"size:255;uniqueIndex;not null" json:"filename"`
	ThumbPath string    `gorm:"size:255" json:"thumb_path"`
	MimeType  string    `gorm:"size:50" json:"mime_type"`
	IsActive  bool      `gorm:"default:true" json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Pack EmoticonPack `gorm:"foreignKey:PackID" json:"pack,omitempty"`
}

func (EmoticonItem) TableName() string {
	return "emoticon_items"
}
