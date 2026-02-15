package v2

import "time"

// V2Memo represents a memo about a member (personal note)
type V2Memo struct { //nolint:revive // V2 prefix for consistency with V2User, V2Post, etc.
	ID           uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	UserID       uint64    `gorm:"column:user_id;index" json:"user_id"`
	TargetUserID uint64    `gorm:"column:target_user_id;index" json:"target_user_id"`
	Content      string    `gorm:"column:content;type:text" json:"content"`
	Color        string    `gorm:"column:color;type:varchar(20);default:'yellow'" json:"color"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (V2Memo) TableName() string { return "v2_memos" }
