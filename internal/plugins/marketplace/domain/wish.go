package domain

import (
	"time"
)

// Wish 찜하기 엔티티
type Wish struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	UserID    uint64    `gorm:"column:user_id;not null;index" json:"user_id"`
	ItemID    uint64    `gorm:"column:item_id;not null;index" json:"item_id"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`

	// Relations
	Item *Item `gorm:"foreignKey:ItemID" json:"item,omitempty"`
}

func (Wish) TableName() string {
	return "marketplace_wishes"
}

// WishItemResponse 찜 목록 응답
type WishItemResponse struct {
	ID        uint64            `json:"id"`
	Item      *ItemListResponse `json:"item"`
	CreatedAt time.Time         `json:"created_at"`
}
