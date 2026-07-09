package domain

import "time"

// MemberBlock represents a member block record.
// Scope 로 차단 범위를 구분한다(#12916): "all"(글+댓글+쪽지), "message"(쪽지만), "content"(글/댓글만).
type MemberBlock struct {
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	MbID        string    `gorm:"column:mb_id;index" json:"mb_id"`
	BlockedMbID string    `gorm:"column:blocked_mb_id;index" json:"blocked_mb_id"`
	Scope       string    `gorm:"column:block_scope;type:varchar(16);default:all;not null" json:"scope"`
	ID          int       `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
}

// Block scope 상수. 미지정/구버전 데이터는 "all" 로 취급한다.
const (
	BlockScopeAll     = "all"
	BlockScopeMessage = "message"
	BlockScopeContent = "content"
)

func (MemberBlock) TableName() string {
	return "g5_member_block"
}

// BlockResponse represents a block item in API responses
type BlockResponse struct {
	UserID    string `json:"user_id"`
	Nickname  string `json:"nickname"`
	BlockedAt string `json:"blocked_at"`
	Scope     string `json:"scope"`
	BlockID   int    `json:"block_id"`
}
