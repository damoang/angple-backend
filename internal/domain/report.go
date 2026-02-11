package domain

import (
	"encoding/json"
	"fmt"
	"time"
)

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
	Hold                     bool       `gorm:"column:hold;default:0" json:"hold"`
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
	AdminDisciplineReasons   string     `gorm:"column:admin_discipline_reasons;type:text" json:"admin_discipline_reasons,omitempty"`
	AdminDisciplineDays      int        `gorm:"column:admin_discipline_days" json:"admin_discipline_days,omitempty"`
	AdminDisciplineType      string     `gorm:"column:admin_discipline_type;size:20" json:"admin_discipline_type,omitempty"`
	AdminDisciplineDetail    string     `gorm:"column:admin_discipline_detail;type:text" json:"admin_discipline_detail,omitempty"`
	Version                  uint       `gorm:"column:version;default:0" json:"version"`
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
	if r.AdminApproved {
		return "scheduled" // 예약 승인 (크론 처리 대기)
	}
	if r.Hold {
		return "hold"
	}
	if r.MonitoringChecked {
		return "monitoring"
	}
	return "pending"
}

// OpinionResponse represents a single opinion in the detail view
type OpinionResponse struct {
	ReviewerID   string `json:"reviewer_id"`
	ReviewerNick string `json:"reviewer_nick"`
	OpinionType  string `json:"opinion_type"`
	Reasons      string `json:"discipline_reasons,omitempty"`
	Days         int    `json:"discipline_days,omitempty"`
	Type         string `json:"discipline_type,omitempty"`
	Detail       string `json:"discipline_detail,omitempty"`
	CreatedAt    string `json:"created_at"`
}

// ProcessResultResponse represents the result of report processing (승인/미조치 결과)
type ProcessResultResponse struct {
	AdminUsers        string   `json:"admin_users,omitempty"`        // 처리 관리자 (없으면 자동처리)
	ProcessedDatetime string   `json:"processed_datetime,omitempty"` // 처리 시각
	DisciplineLogID   int      `json:"discipline_log_id,omitempty"`  // 징계 로그 ID
	PenaltyDays       int      `json:"penalty_days"`                 // 0=주의, -1=영구
	PenaltyType       []string `json:"penalty_type,omitempty"`       // ["level", "access"]
	PenaltyReasons    []string `json:"penalty_reasons,omitempty"`    // 사유 문자열 키
	SgTypes           []int    `json:"sg_types,omitempty"`           // 사유 정수 코드
	IsBulk            bool     `json:"is_bulk"`                      // 벌크 처리 여부
	ReportCount       int      `json:"report_count"`                 // 묶인 건수
	AdminMemo         string   `json:"admin_memo,omitempty"`         // 관리자 메모
}

// ReportDetailResponse is the enhanced detail response
type ReportDetailResponse struct {
	Report        ReportListResponse    `json:"report"`
	AllReports    []ReportListResponse  `json:"all_reports"`
	Opinions      []OpinionResponse     `json:"opinions"`
	Status        string                `json:"status"`
	ProcessResult *ProcessResultResponse `json:"process_result,omitempty"` // 처리 결과 (승인/미조치 시에만)
}

// ReportDetailEnhancedResponse extends ReportDetailResponse with optional data (Phase 2: 통합 API)
type ReportDetailEnhancedResponse struct {
	ReportDetailResponse
	AIEvaluations    []AIEvaluation   `json:"ai_evaluations,omitempty"`    // AI 평가 목록 (?include=ai)
	DisciplineHistory []DisciplineLog `json:"discipline_history,omitempty"` // 징계 이력 (?include=history)
}

// ReportListResponse represents report list response
type ReportListResponse struct {
	ID                int    `json:"id"`
	Table             string `json:"table"`
	Parent            int    `json:"parent"`
	Type              int8   `json:"type"`               // 1=post, 2=comment
	BoardSubject      string `json:"bo_subject"`          // 게시판 이름
	ReporterID        string `json:"reporter_id"`
	ReporterNickname  string `json:"reporter_nickname"`   // 신고자 닉네임
	TargetID          string `json:"target_id"`
	TargetNickname    string `json:"target_nickname"`     // 피신고자 닉네임
	TargetTitle       string `json:"target_title"`        // 신고 대상 글 제목
	TargetContent     string `json:"target_content"`      // 신고 대상 본문 미리보기
	Reason            string `json:"reason"`
	Status            string `json:"status"`
	CreatedAt         string `json:"created_at"`
	AdminUsers        string `json:"admin_users,omitempty"`
	ProcessedDatetime string `json:"processed_datetime,omitempty"`
}

// AggregatedReportResponse represents an aggregated report group for list view
type AggregatedReportResponse struct {
	Table             string            `json:"table"`
	SGID              int               `json:"sg_id"`
	Parent            int               `json:"parent"`
	ReportCount       int               `json:"report_count"`
	ReporterCount     int               `json:"reporter_count"`
	TargetID          string            `json:"target_id"`
	TargetNickname    string            `json:"target_nickname"`
	TargetTitle       string            `json:"target_title"`
	TargetContent     string            `json:"target_content"`
	BoardSubject      string            `json:"bo_subject"`
	ReportTypes       string            `json:"report_types"`
	OpinionCount      int               `json:"opinion_count"`
	ActionCount       int               `json:"action_count"`
	DismissCount      int               `json:"dismiss_count"`
	Status            string            `json:"status"`
	FirstReportTime   string            `json:"first_report_time"`
	LatestReportTime  string            `json:"latest_report_time"`
	ReviewerIDs       []string          `json:"reviewer_ids,omitempty"`
	ReviewedCount     int               `json:"reviewed_count"`
	TotalReviewers    int               `json:"total_reviewers"`
	ReviewedByMe      bool              `json:"reviewed_by_me"`
	Opinions          []OpinionResponse `json:"opinions,omitempty"`
	AdminUsers        string            `json:"admin_users,omitempty"`
	ProcessedDatetime string            `json:"processed_datetime,omitempty"`
}

// TargetAggregatedResponse represents reports grouped by target user (피신고자별 그룹핑)
type TargetAggregatedResponse struct {
	TargetID         string                     `json:"target_id"`
	TargetNickname   string                     `json:"target_nickname"`
	ReportCount      int                        `json:"report_count"`       // 전체 신고 건수
	ContentCount     int                        `json:"content_count"`      // 신고된 콘텐츠(글/댓글) 수
	ReporterCount    int                        `json:"reporter_count"`     // 고유 신고자 수
	LatestReportTime string                     `json:"latest_report_time"` // 가장 최근 신고 시각
	FirstReportTime  string                     `json:"first_report_time"`  // 최초 신고 시각
	DisciplineCount  int                        `json:"discipline_count"`   // 기존 이용제한 이력 건수
	Contents         []AggregatedReportResponse `json:"contents,omitempty"` // 하위 콘텐츠 목록 (펼침용)
}

// TargetAggregatedRow represents raw DB row for target-user aggregation
type TargetAggregatedRow struct {
	TargetID         string `gorm:"column:target_mb_id"`
	ReportCount      int    `gorm:"column:report_count"`
	ContentCount     int    `gorm:"column:content_count"`
	ReporterCount    int    `gorm:"column:reporter_count"`
	LatestReportTime string `gorm:"column:latest_report_time"`
	FirstReportTime  string `gorm:"column:first_report_time"`
}

// SubmitReportRequest represents a user submitting a report
type SubmitReportRequest struct {
	TargetID string `json:"target_id" binding:"required"` // 신고 대상 회원 ID
	Table    string `json:"table" binding:"required"`     // 게시판 테이블명 (예: free, qa)
	PostID   int    `json:"post_id" binding:"required"`   // 게시글 ID
	Reason   string `json:"reason" binding:"required"`    // 신고 사유
}

// BatchReportActionRequest represents a batch processing request
type BatchReportActionRequest struct {
	Action         string   `json:"action"`
	Tables         []string `json:"tables"`
	Parents        []int    `json:"parents"`
	AdminMemo      string   `json:"adminMemo,omitempty"`
	PenaltyDays    int      `json:"penalty_days,omitempty"`
	PenaltyType    []string `json:"penalty_type,omitempty"`
	PenaltyReasons []string `json:"penalty_reasons,omitempty"`
	Immediate      bool     `json:"immediate,omitempty"` // true=즉시, false=예약(기본)
}

// ReportActionRequest represents request for report action
type ReportActionRequest struct {
	Action  string   `json:"action"` // submitOpinion, cancelOpinion, adminApprove, adminDismiss, adminHold
	Table   string   `json:"sg_table"`
	SGID    int      `json:"sg_id"`
	Parent  int      `json:"sg_parent"`
	Reasons []string `json:"reasons,omitempty"`
	Days    int      `json:"days,omitempty"`
	Type    string   `json:"type,omitempty"`
	Detail  string   `json:"detail,omitempty"`
	// Frontend fields (singo 앱에서 전송하는 필드)
	ReportID       int      `json:"id,omitempty"`              // 신고 primary key (g5_na_singo.id)
	Opinion        string   `json:"opinion,omitempty"`         // 의견: action | no_action
	OpinionText    string   `json:"opinionText,omitempty"`     // 의견 상세 텍스트
	AdminMemo      string   `json:"adminMemo,omitempty"`       // 관리자 메모
	PenaltyDays    int      `json:"penalty_days,omitempty"`    // 제한 일수: 0=주의, 9999=영구
	PenaltyType    []string `json:"penalty_type,omitempty"`    // ["level", "intercept"]
	PenaltyReasons []string `json:"penalty_reasons,omitempty"` // 사유 코드 (21-40)
	Immediate      bool     `json:"immediate,omitempty"`       // true=즉시 실행, false=예약 실행(PHP 크론 처리)
	Version        *uint    `json:"version,omitempty"`         // Phase 6-2: Optimistic Locking용
}

// AdminApproval represents a single admin approval entry in admin_users JSON field.
// IMPORTANT: admin_users must be stored as JSON array format: [{"mb_id":"admin123","datetime":"2025-01-21 12:00:00"}]
// Compatible with PHP SingoHelper::addAdminApproval()
type AdminApproval struct {
	MbID     string `json:"mb_id"`
	Datetime string `json:"datetime"`
}

// AdminApprovalList is a slice of AdminApproval entries
type AdminApprovalList []AdminApproval

// AddAdminApproval adds a new approval to existing admin_users JSON.
// If existingJSON is empty or invalid, starts fresh with a new array.
// Prevents duplicate entries (same mb_id).
// Returns the new JSON string in format: [{"mb_id":"...","datetime":"..."}]
func AddAdminApproval(existingJSON, adminID string) (string, error) {
	approvals, err := ParseAdminUsers(existingJSON)
	if err != nil {
		// Start fresh on parse error (handles plain text like "admin123")
		approvals = AdminApprovalList{}
	}

	// Check for duplicate
	for _, approval := range approvals {
		if approval.MbID == adminID {
			return existingJSON, nil // Already approved by this admin
		}
	}

	// Add new approval with current timestamp
	now := time.Now().Format("2006-01-02 15:04:05")
	approvals = append(approvals, AdminApproval{
		MbID:     adminID,
		Datetime: now,
	})

	data, err := json.Marshal(approvals)
	if err != nil {
		return "", fmt.Errorf("failed to marshal admin_users: %w", err)
	}
	return string(data), nil
}

// ParseAdminUsers parses admin_users JSON string into AdminApprovalList.
// Returns empty slice for empty string.
// Returns error if JSON is invalid (e.g., plain text like "admin123").
func ParseAdminUsers(jsonStr string) (AdminApprovalList, error) {
	if jsonStr == "" {
		return AdminApprovalList{}, nil
	}

	var approvals AdminApprovalList
	if err := json.Unmarshal([]byte(jsonStr), &approvals); err != nil {
		return nil, fmt.Errorf("invalid admin_users JSON format (expected array): %w", err)
	}
	return approvals, nil
}
