package domain

import "time"

// Notification represents a user notification (알림)
// Based on g5_na_noti table (nariya plugin)
type Notification struct {
	ID            int       `gorm:"column:ph_id;primaryKey;autoIncrement" json:"id"`
	ToCase        string    `gorm:"column:ph_to_case" json:"to_case"`                // board, comment, inquire
	FromCase      string    `gorm:"column:ph_from_case" json:"from_case"`            // write, board, comment, good, nogood, answer, reply
	BoardTable    string    `gorm:"column:bo_table" json:"board_table"`              // 게시판 테이블
	RelBoardTable string    `gorm:"column:rel_bo_table" json:"rel_board_table"`      // 관련 게시판 테이블
	WriteID       int       `gorm:"column:wr_id" json:"write_id"`                    // 게시글 ID
	RelWriteID    int       `gorm:"column:rel_wr_id" json:"rel_write_id"`            // 관련 게시글 ID
	MemberID      string    `gorm:"column:mb_id;index" json:"member_id"`             // 수신자 회원 ID
	SenderID      string    `gorm:"column:rel_mb_id" json:"sender_id,omitempty"`     // 발신자 회원 ID
	SenderName    string    `gorm:"column:rel_mb_nick" json:"sender_name,omitempty"` // 발신자 닉네임
	Message       string    `gorm:"column:rel_msg" json:"message"`                   // 메시지
	URL           string    `gorm:"column:rel_url" json:"url,omitempty"`             // URL
	IsReadChar    string    `gorm:"column:ph_readed" json:"-"`                       // 'Y' or 'N'
	CreatedAt     time.Time `gorm:"column:ph_datetime" json:"created_at"`            // 생성일시
	Title         string    `gorm:"column:parent_subject" json:"title"`              // 제목 (parent_subject)
	WriteParent   int       `gorm:"column:wr_parent" json:"write_parent"`            // 부모 게시글 ID
}

// IsRead returns whether the notification has been read
func (n *Notification) IsRead() bool {
	return n.IsReadChar == "Y"
}

// SetRead sets the read status
func (n *Notification) SetRead(read bool) {
	if read {
		n.IsReadChar = "Y"
	} else {
		n.IsReadChar = "N"
	}
}

// Type returns the notification type (alias for FromCase for API compatibility)
func (n *Notification) Type() string {
	return n.FromCase
}

// Content returns the notification content (alias for Message for API compatibility)
func (n *Notification) Content() string {
	return n.Message
}

// TableName returns the table name
func (Notification) TableName() string {
	return "g5_na_noti"
}

// NotificationSummaryResponse represents unread count response
type NotificationSummaryResponse struct {
	TotalUnread int `json:"total_unread"`
}

// NotificationListResponse represents notification list response
type NotificationListResponse struct {
	Items       []NotificationItem `json:"items"`
	Total       int64              `json:"total"`
	UnreadCount int64              `json:"unread_count"`
	Page        int                `json:"page"`
	Limit       int                `json:"limit"`
	TotalPages  int                `json:"total_pages"`
}

// NotificationItem represents a single notification in list
type NotificationItem struct {
	ID         int    `json:"id"`
	Type       string `json:"type"`
	Title      string `json:"title"`
	Content    string `json:"content"`
	URL        string `json:"url,omitempty"`
	SenderID   string `json:"sender_id,omitempty"`
	SenderName string `json:"sender_name,omitempty"`
	IsRead     bool   `json:"is_read"`
	CreatedAt  string `json:"created_at"`
}
