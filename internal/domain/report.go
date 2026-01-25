package domain

import "time"

// Report represents a user report (신고) - maps to g5_na_singo table
type Report struct {
	ID                       int        `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Flag                     int8       `gorm:"column:sg_flag" json:"flag"` // 0: pending, 1: monitoring, 2: approved, 3: dismissed
	ReporterID               string     `gorm:"column:mb_id" json:"reporter_id"`
	Table                    string     `gorm:"column:sg_table" json:"table"`
	SGID                     int        `gorm:"column:sg_id" json:"sg_id"`
	Parent                   int        `gorm:"column:sg_parent" json:"parent"`
	Type                     int8       `gorm:"column:sg_type" json:"type"`
	Reason                   string     `gorm:"column:sg_desc" json:"reason"`
	WriteTime                time.Time  `gorm:"column:wr_time" json:"write_time"`
	CreatedAt                time.Time  `gorm:"column:sg_time" json:"created_at"`
	IP                       string     `gorm:"column:sg_ip" json:"ip"`
	MonitoringUsers          string     `gorm:"column:monitoring_users" json:"monitoring_users,omitempty"`
	MonitoringChecked        bool       `gorm:"column:monitoring_checked" json:"monitoring_checked"`
	MonitoringDatetime       *time.Time `gorm:"column:monitoring_datetime" json:"monitoring_datetime,omitempty"`
	MonitoringDiscipline     string     `gorm:"column:monitoring_discipline_reasons" json:"monitoring_discipline_reasons,omitempty"`
	MonitoringDisciplineDays *int       `gorm:"column:monitoring_discipline_days" json:"monitoring_discipline_days,omitempty"`
	MonitoringDisciplineType string     `gorm:"column:monitoring_discipline_type" json:"monitoring_discipline_type,omitempty"`
	AdminUsers               string     `gorm:"column:admin_users" json:"admin_users,omitempty"`
	AdminApproved            bool       `gorm:"column:admin_approved" json:"admin_approved"`
	AdminDatetime            *time.Time `gorm:"column:admin_datetime" json:"admin_datetime,omitempty"`
	Processed                bool       `gorm:"column:processed" json:"processed"`
	ProcessedDatetime        *time.Time `gorm:"column:processed_datetime" json:"processed_datetime,omitempty"`
	DisciplineLogID          *int       `gorm:"column:discipline_log_id" json:"discipline_log_id,omitempty"`
	TargetID                 string     `gorm:"column:target_mb_id" json:"target_id,omitempty"`
	TargetContent            string     `gorm:"column:target_content" json:"target_content,omitempty"`
	TargetTitle              string     `gorm:"column:target_title" json:"target_title,omitempty"`
}

// TableName returns the table name
func (Report) TableName() string {
	return "g5_na_singo"
}

// Status returns the status string based on flag
func (r *Report) Status() string {
	if r.Processed {
		if r.AdminApproved {
			return "approved"
		}
		return "dismissed"
	}
	if r.MonitoringChecked {
		return "monitoring"
	}
	return "pending"
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
