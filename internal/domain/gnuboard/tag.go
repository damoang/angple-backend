package gnuboard

import "time"

// G5NaTag represents a tag in g5_na_tag table (nariya tag system)
type G5NaTag struct {
	ID       int       `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Type     int8      `gorm:"column:type;default:0" json:"type"`
	Idx      string    `gorm:"column:idx;type:varchar(10)" json:"idx"`
	Tag      string    `gorm:"column:tag;type:varchar(255)" json:"tag"`
	Cnt      int       `gorm:"column:cnt;default:0" json:"cnt"`
	RegDate  time.Time `gorm:"column:regdate" json:"regdate"`
	LastDate time.Time `gorm:"column:lastdate" json:"lastdate"`
}

// TableName returns the table name
func (G5NaTag) TableName() string { return "g5_na_tag" }

// G5NaTagLog represents a post-tag mapping in g5_na_tag_log table
type G5NaTagLog struct {
	ID      int       `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	BoTable string    `gorm:"column:bo_table;type:varchar(20)" json:"bo_table"`
	WrID    int       `gorm:"column:wr_id" json:"wr_id"`
	TagID   int       `gorm:"column:tag_id" json:"tag_id"`
	Tag     string    `gorm:"column:tag;type:varchar(255)" json:"tag"`
	MbID    string    `gorm:"column:mb_id;type:varchar(255)" json:"mb_id"`
	RegDate time.Time `gorm:"column:regdate" json:"regdate"`
}

// TableName returns the table name
func (G5NaTagLog) TableName() string { return "g5_na_tag_log" }
