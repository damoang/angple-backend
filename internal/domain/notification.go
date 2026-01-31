package domain

import "time"

// Notification represents a user notification (알림)
type Notification struct {
	ID         int       `gorm:"column:nt_id;primaryKey;autoIncrement" json:"id"`
	MemberID   string    `gorm:"column:mb_id;index" json:"member_id"`
	Type       string    `gorm:"column:nt_type" json:"type"`
	Title      string    `gorm:"column:nt_title" json:"title"`
	Content    string    `gorm:"column:nt_content" json:"content"`
	URL        string    `gorm:"column:nt_url" json:"url,omitempty"`
	SenderID   string    `gorm:"column:nt_sender_id" json:"sender_id,omitempty"`
	SenderName string    `gorm:"column:nt_sender_name" json:"sender_name,omitempty"`
	IsRead     bool      `gorm:"column:nt_is_read" json:"is_read"`
	CreatedAt  time.Time `gorm:"column:nt_created_at" json:"created_at"`
}

// TableName returns the table name
func (Notification) TableName() string {
	return "g5_da_notification"
}

// NotificationSummaryResponse represents unread count response
type NotificationSummaryResponse struct {
	TotalUnread int `json:"total_unread"`
}

// NotificationListResponse represents notification list response
type NotificationListResponse struct {
	Items      []NotificationItem `json:"items"`
	Total      int64              `json:"total"`
	UnreadCount int64             `json:"unread_count"`
	Page       int                `json:"page"`
	Limit      int                `json:"limit"`
	TotalPages int                `json:"total_pages"`
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
