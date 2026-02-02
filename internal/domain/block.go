package domain

import "time"

// MemberBlock represents a member block record
type MemberBlock struct {
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	MbID        string    `gorm:"column:mb_id;index" json:"mb_id"`
	BlockedMbID string    `gorm:"column:blocked_mb_id;index" json:"blocked_mb_id"`
	ID          int       `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
}

func (MemberBlock) TableName() string {
	return "g5_member_block"
}

// BlockResponse represents a block item in API responses
type BlockResponse struct {
	UserID    string `json:"user_id"`
	Nickname  string `json:"nickname"`
	BlockedAt string `json:"blocked_at"`
	BlockID   int    `json:"block_id"`
}
