package gnuboard

import "time"

const AnniversaryEventCode2026 = "2026_2nd_anniversary"

// AnniversaryDrawEntry stores one draw participation per member for the anniversary event.
type AnniversaryDrawEntry struct {
	ID          uint64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	EventCode   string     `gorm:"column:event_code" json:"event_code"`
	MbID        string     `gorm:"column:mb_id" json:"mb_id"`
	DrawResult  string     `gorm:"column:draw_result" json:"draw_result"`
	PointAmount int        `gorm:"column:point_amount" json:"point_amount"`
	GrantedAt   *time.Time `gorm:"column:granted_at" json:"granted_at,omitempty"`
	PointPoID   *int       `gorm:"column:point_po_id" json:"point_po_id,omitempty"`
	CreatedAt   time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt   time.Time  `gorm:"column:updated_at" json:"updated_at"`
}

func (AnniversaryDrawEntry) TableName() string {
	return "event_anniversary_draw_entries"
}
