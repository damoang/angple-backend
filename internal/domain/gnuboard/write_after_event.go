package gnuboard

import "time"

const (
	WriteAfterEventTypePostCreated            = "post_created"
	WriteAfterEventTypeCommentCreated         = "comment_created"
	WriteAfterEventTypePostUpdated            = "post_updated"
	WriteAfterEventTypeCommentUpdated         = "comment_updated"
	WriteAfterEventTypePostDeleted            = "post_deleted"
	WriteAfterEventTypeCommentDeleted         = "comment_deleted"
	WriteAfterEventTypePostRestored           = "post_restored"
	WriteAfterEventTypeCommentRestored        = "comment_restored"
	WriteAfterEventTypeAffiliatePostSync      = "affiliate_post_sync"
	WriteAfterEventTypeAffiliateCommentSync   = "affiliate_comment_sync"
	WriteAfterEventTypeAffiliatePostDelete    = "affiliate_post_delete"
	WriteAfterEventTypeAffiliateCommentDelete = "affiliate_comment_delete"

	WriteAfterEventStatusPending    = "pending"
	WriteAfterEventStatusProcessing = "processing"
	WriteAfterEventStatusProcessed  = "processed"
	// WriteAfterEventStatusDead 는 재시도 상한을 넘겨 더 이상 시도하지 않는 종착 상태다.
	// ⛔ 상한이 없으면 영구 실패 이벤트(원글 삭제 등)가 큐에 남아 **영원히 재시도**한다.
	//    2026-08-21 실측: retry_count 최대 1,961회. 17.9만 건이 시간당 재시도하며
	//    오리진·CDN 을 3중으로 물었다.
	WriteAfterEventStatusDead = "dead"

	// WriteAfterEventMaxRetry 는 재시도 상한이다. 제휴 이벤트는 1회→1분, 2회→10분,
	// 그 이상 1시간 간격이므로 100회면 약 4일 재시도한 뒤 포기하는 셈이다.
	WriteAfterEventMaxRetry = 100
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
