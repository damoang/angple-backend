package gnuboard

import "time"

const (
	WriteAfterEventTypePostCreated     = "post_created"
	WriteAfterEventTypeCommentCreated  = "comment_created"
	WriteAfterEventTypePostUpdated     = "post_updated"
	WriteAfterEventTypeCommentUpdated  = "comment_updated"
	WriteAfterEventTypePostDeleted     = "post_deleted"
	WriteAfterEventTypeCommentDeleted  = "comment_deleted"
	WriteAfterEventTypePostRestored    = "post_restored"
	WriteAfterEventTypeCommentRestored = "comment_restored"

	WriteAfterEventStatusPending    = "pending"
	WriteAfterEventStatusProcessing = "processing"
	WriteAfterEventStatusProcessed  = "processed"
)

type WriteAfterEvent struct {
	ID          int64      `gorm:"column:id;primaryKey" json:"id"`
	EventType   string     `gorm:"column:event_type" json:"event_type"`
	BoardSlug   string     `gorm:"column:board_slug" json:"board_slug"`
	WriteID     int        `gorm:"column:write_id" json:"write_id"`
	PostID      *int       `gorm:"column:post_id" json:"post_id,omitempty"`
	ParentID    *int       `gorm:"column:parent_id" json:"parent_id,omitempty"`
	MemberID    string     `gorm:"column:member_id" json:"member_id"`
	Author      string     `gorm:"column:author" json:"author"`
	Subject     string     `gorm:"column:subject" json:"subject"`
	OccurredAt  time.Time  `gorm:"column:occurred_at" json:"occurred_at"`
	AvailableAt time.Time  `gorm:"column:available_at" json:"available_at"`
	Status      string     `gorm:"column:status" json:"status"`
	RetryCount  int        `gorm:"column:retry_count" json:"retry_count"`
	LastError   *string    `gorm:"column:last_error" json:"last_error,omitempty"`
	ClaimedAt   *time.Time `gorm:"column:claimed_at" json:"claimed_at,omitempty"`
	ProcessedAt *time.Time `gorm:"column:processed_at" json:"processed_at,omitempty"`
	CreatedAt   time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt   time.Time  `gorm:"column:updated_at" json:"updated_at"`
}

func (WriteAfterEvent) TableName() string {
	return "g5_write_after_events"
}
