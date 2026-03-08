package gnuboard

import "time"

// ScheduledDelete represents a pending deletion in g5_scheduled_deletes
type ScheduledDelete struct {
	ID           int64      `gorm:"column:id;primaryKey" json:"id"`
	BoTable      string     `gorm:"column:bo_table" json:"bo_table"`
	WrID         int        `gorm:"column:wr_id" json:"wr_id"`
	WrIsComment  int        `gorm:"column:wr_is_comment" json:"wr_is_comment"`
	ReplyCount   int        `gorm:"column:reply_count" json:"reply_count"`
	DelayMinutes int        `gorm:"column:delay_minutes" json:"delay_minutes"`
	ScheduledAt  time.Time  `gorm:"column:scheduled_at" json:"scheduled_at"`
	RequestedBy  string     `gorm:"column:requested_by" json:"requested_by"`
	RequestedAt  time.Time  `gorm:"column:requested_at" json:"requested_at"`
	CancelledAt  *time.Time `gorm:"column:cancelled_at" json:"cancelled_at,omitempty"`
	ExecutedAt   *time.Time `gorm:"column:executed_at" json:"executed_at,omitempty"`
	Status       string     `gorm:"column:status" json:"status"`
}

// TableName returns the table name for GORM
func (ScheduledDelete) TableName() string {
	return "g5_scheduled_deletes"
}

// CalculateDelay returns the delay in minutes based on comment/reply count
func CalculateDelay(replyCount int) int {
	switch {
	case replyCount == 0:
		return 0
	case replyCount <= 4:
		return 5
	case replyCount <= 9:
		return 30
	case replyCount <= 29:
		return 120
	case replyCount <= 99:
		return 360
	default:
		return 1440
	}
}
