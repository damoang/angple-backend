package domain

import "time"

// ContentHistory represents a content edit/delete history record from g5_da_content_history
type ContentHistory struct {
	ID           uint      `gorm:"column:id;primaryKey" json:"id"`
	BoTable      string    `gorm:"column:bo_table" json:"bo_table"`
	WrID         uint      `gorm:"column:wr_id" json:"wr_id"`
	WrIsComment  int8      `gorm:"column:wr_is_comment" json:"wr_is_comment"`
	MbID         string    `gorm:"column:mb_id" json:"mb_id"`
	WrName       string    `gorm:"column:wr_name" json:"wr_name"`
	Operation    string    `gorm:"column:operation" json:"operation"`
	OperatedBy   string    `gorm:"column:operated_by" json:"operated_by"`
	OperatedAt   time.Time `gorm:"column:operated_at" json:"operated_at"`
	PreviousData string    `gorm:"column:previous_data;type:json" json:"previous_data,omitempty"`
}

func (ContentHistory) TableName() string {
	return "g5_da_content_history"
}
