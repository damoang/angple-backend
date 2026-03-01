package gnuboard

import "time"

// G5NaXP represents the g5_na_xp table (Nariya experience points log)
type G5NaXP struct {
	XpID        int       `gorm:"column:xp_id;primaryKey;autoIncrement" json:"xp_id"`
	MbID        string    `gorm:"column:mb_id;index" json:"mb_id"`
	XpDatetime  time.Time `gorm:"column:xp_datetime" json:"xp_datetime"`
	XpContent   string    `gorm:"column:xp_content" json:"xp_content"`
	XpPoint     int       `gorm:"column:xp_point" json:"xp_point"`
	XpRelTable  string    `gorm:"column:xp_rel_table" json:"xp_rel_table"`
	XpRelID     string    `gorm:"column:xp_rel_id" json:"xp_rel_id"`
	XpRelAction string    `gorm:"column:xp_rel_action" json:"xp_rel_action"`
}

// TableName returns the table name for GORM
func (G5NaXP) TableName() string {
	return "g5_na_xp"
}

// ExpHistory represents an experience history item for API response
type ExpHistory struct {
	ID        int       `json:"id"`
	Content   string    `json:"content"`
	Point     int       `json:"point"`
	RelTable  string    `json:"rel_table"`
	RelID     string    `json:"rel_id"`
	Action    string    `json:"action"`
	CreatedAt time.Time `json:"created_at"`
}

// ToExpHistory converts G5NaXP to ExpHistory
func (x *G5NaXP) ToExpHistory() ExpHistory {
	return ExpHistory{
		ID:        x.XpID,
		Content:   x.XpContent,
		Point:     x.XpPoint,
		RelTable:  x.XpRelTable,
		RelID:     x.XpRelID,
		Action:    x.XpRelAction,
		CreatedAt: x.XpDatetime,
	}
}
