package domain

import "time"

// MemberMemo represents a member memo (회원 메모)
type MemberMemo struct {
	ID              int        `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	MemberUID       int        `gorm:"column:member_uid" json:"member_uid"`
	MemberID        string     `gorm:"column:member_id;index" json:"member_id"`
	TargetMemberUID int        `gorm:"column:target_member_uid" json:"target_member_uid"`
	TargetMemberID  string     `gorm:"column:target_member_id;index" json:"target_member_id"`
	Memo            string     `gorm:"column:memo;size:255" json:"memo"`
	MemoDetail      string     `gorm:"column:memo_detail;type:text" json:"memo_detail"`
	Color           string     `gorm:"column:color;size:50" json:"color"`
	CreatedAt       time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt       *time.Time `gorm:"column:updated_at" json:"updated_at,omitempty"`
}

// TableName returns the table name
func (MemberMemo) TableName() string {
	return "g5_member_memo"
}

// MemoRequest represents request for creating/updating memo
type MemoRequest struct {
	TargetID   string `json:"target_id" binding:"required"`
	Content    string `json:"content"`     // maps to memo field
	MemoDetail string `json:"memo_detail"` // detailed memo
	Color      string `json:"color"`       // color for display (default: yellow)
}

// MemoResponse represents memo response
type MemoResponse struct {
	TargetID       string `json:"target_id,omitempty"`
	TargetNickname string `json:"target_nickname,omitempty"`
	Content        string `json:"content,omitempty"`
	MemoDetail     string `json:"memo_detail,omitempty"`
	Color          string `json:"color,omitempty"`
	Token          string `json:"_token,omitempty"`
	CreatedAt      string `json:"created_at,omitempty"`
	UpdatedAt      string `json:"updated_at,omitempty"`
}
