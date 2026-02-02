package v2

import "time"

// V2Scrap represents a user's scrap (bookmark) of a post
type V2Scrap struct { //nolint:revive // V2 prefix for consistency with V2User, V2Post, etc.
	ID        uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	UserID    uint64    `gorm:"column:user_id;index" json:"user_id"`
	PostID    uint64    `gorm:"column:post_id;index" json:"post_id"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (V2Scrap) TableName() string { return "v2_scraps" }
