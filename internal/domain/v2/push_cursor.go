package v2

import "time"

// V2PushCursor is a single-row watermark for the push notification outbox poller.
// Row id=1 holds the last g5_na_noti.ph_id that has been processed for push.
type V2PushCursor struct {
	ID        uint64    `gorm:"column:id;primaryKey" json:"id"`
	LastPhID  int       `gorm:"column:last_ph_id;not null;default:0" json:"last_ph_id"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (V2PushCursor) TableName() string { return "v2_push_cursors" }
