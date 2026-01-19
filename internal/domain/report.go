package domain

import "time"

// Report represents a user report (신고)
type Report struct {
	ID          int       `gorm:"column:sg_id;primaryKey;autoIncrement" json:"id"`
	Table       string    `gorm:"column:sg_table;size:50;index" json:"table"`
	Parent      int       `gorm:"column:sg_parent" json:"parent"`
	ReporterID  string    `gorm:"column:mb_id;index" json:"reporter_id"`
	TargetID    string    `gorm:"column:target_mb_id;index" json:"target_id"`
	Reason      string    `gorm:"column:sg_reason;type:text" json:"reason"`
	Status      string    `gorm:"column:sg_status;size:20" json:"status"` // pending, monitoring, approved, dismissed
	CreatedAt   time.Time `gorm:"column:sg_datetime" json:"created_at"`
	ProcessedAt time.Time `gorm:"column:sg_processed_at" json:"processed_at,omitempty"`
	ProcessedBy string    `gorm:"column:sg_processed_by" json:"processed_by,omitempty"`
}

// TableName returns the table name
func (Report) TableName() string {
	return "g5_singo"
}

// ReportListResponse represents report list response
type ReportListResponse struct {
	ID         int    `json:"id"`
	Table      string `json:"table"`
	Parent     int    `json:"parent"`
	ReporterID string `json:"reporter_id"`
	TargetID   string `json:"target_id"`
	Reason     string `json:"reason"`
	Status     string `json:"status"`
	CreatedAt  string `json:"created_at"`
}

// ReportActionRequest represents request for report action
type ReportActionRequest struct {
	Action  string   `json:"action"` // submitOpinion, cancelOpinion, adminApprove, adminDismiss
	Table   string   `json:"sg_table"`
	ID      int      `json:"sg_id"`
	Parent  int      `json:"sg_parent"`
	Reasons []string `json:"reasons,omitempty"`
	Days    int      `json:"days,omitempty"`
	Type    string   `json:"type,omitempty"`
	Detail  string   `json:"detail,omitempty"`
}
