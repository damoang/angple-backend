package domain

import "time"

// Opinion represents a monitoring opinion on a report - maps to g5_na_singo_opinions table
type Opinion struct {
	ID                int       `gorm:"primaryKey;autoIncrement" json:"id"`
	Table             string    `gorm:"column:sg_table;size:50" json:"sg_table"`
	SGID              int       `gorm:"column:sg_id" json:"sg_id"`
	Parent            int       `gorm:"column:sg_parent" json:"sg_parent"`
	ReviewerID        string    `gorm:"column:reviewer_id;size:50" json:"reviewer_id"`
	OpinionType       string    `gorm:"column:opinion_type;size:10" json:"opinion_type"` // action|dismiss
	DisciplineReasons string    `gorm:"column:discipline_reasons;type:text" json:"discipline_reasons"`
	DisciplineDays    int       `gorm:"column:discipline_days" json:"discipline_days"`
	DisciplineType    string    `gorm:"column:discipline_type;size:20" json:"discipline_type"`
	DisciplineDetail  string    `gorm:"column:discipline_detail;type:text" json:"discipline_detail"`
	CreatedAt         time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt         time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// TableName returns the table name
func (Opinion) TableName() string {
	return "g5_na_singo_opinions"
}
