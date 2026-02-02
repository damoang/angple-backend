package domain

import "time"

// Scrap represents a bookmarked post (g5_scrap table)
type Scrap struct {
	DateTime time.Time `gorm:"column:ms_datetime" json:"datetime"`
	MbID     string    `gorm:"column:mb_id;index" json:"mb_id"`
	BoTable  string    `gorm:"column:bo_table" json:"bo_table"`
	WrID     int       `gorm:"column:wr_id" json:"wr_id"`
	ID       int       `gorm:"column:ms_id;primaryKey;autoIncrement" json:"id"`
}

func (Scrap) TableName() string {
	return "g5_scrap"
}

// ScrapResponse represents a scrap item in API responses
type ScrapResponse struct {
	CreatedAt string `json:"created_at"`
	BoardID   string `json:"board_id"`
	Title     string `json:"title"`
	Author    string `json:"author"`
	ScrapID   int    `json:"scrap_id"`
	PostID    int    `json:"post_id"`
}
