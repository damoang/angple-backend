package v2

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

// DisciplineLog represents a disciplinary action record
type DisciplineLog struct {
	ID              uint64           `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	MemberID        string           `gorm:"column:member_id;type:varchar(50);index" json:"member_id"`
	MemberNickname  string           `gorm:"column:member_nickname;type:varchar(100)" json:"member_nickname"`
	PenaltyPeriod   int              `gorm:"column:penalty_period" json:"penalty_period"` // -1: permanent, 0: warning, >0: days
	PenaltyDateFrom time.Time        `gorm:"column:penalty_date_from" json:"penalty_date_from"`
	PenaltyDateTo   *time.Time       `gorm:"column:penalty_date_to" json:"penalty_date_to,omitempty"`
	ViolationTypes  IntArray         `gorm:"column:violation_types;type:json" json:"violation_types"`
	ReportedItems   ReportedItemList `gorm:"column:reported_items;type:json" json:"reported_items,omitempty"`
	CreatedBy       string           `gorm:"column:created_by;type:varchar(50)" json:"created_by"`
	CreatedAt       time.Time        `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	Status          string           `gorm:"column:status;type:enum('pending','approved','rejected');default:'approved'" json:"status"`
}

// TableName returns the table name
func (DisciplineLog) TableName() string { return "v2_discipline_logs" }

// ReportedItem represents a reported post or comment
type ReportedItem struct {
	BoardID  string `json:"board_id"`
	PostID   int    `json:"post_id"`
	ParentID int    `json:"parent_id,omitempty"`
}

// IntArray is a custom type for JSON array of integers
type IntArray []int

// Scan implements sql.Scanner
func (a *IntArray) Scan(value interface{}) error {
	if value == nil {
		*a = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(bytes, a)
}

// Value implements driver.Valuer
func (a IntArray) Value() (driver.Value, error) {
	if a == nil {
		return nil, nil
	}
	return json.Marshal(a)
}

// ReportedItemList is a custom type for JSON array of ReportedItem
type ReportedItemList []ReportedItem

// Scan implements sql.Scanner
func (r *ReportedItemList) Scan(value interface{}) error {
	if value == nil {
		*r = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(bytes, r)
}

// Value implements driver.Valuer
func (r ReportedItemList) Value() (driver.Value, error) {
	if r == nil {
		return nil, nil
	}
	return json.Marshal(r)
}

// ViolationType represents a type of rule violation
type ViolationType struct {
	Code        int    `json:"code"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// ViolationTypes is the list of all violation types (18 types from PHP)
var ViolationTypes = []ViolationType{
	{1, "회원비하", "회원을 비난하거나 비하하는 행위"},
	{2, "예의없음", "반말 등 예의를 갖추지 않은 행위"},
	{3, "부적절한 표현", "욕설, 비속어, 혐오표현 등"},
	{4, "차별행위", "지역, 세대, 성, 인종 등 차별"},
	{5, "정치관련", "정당, 정책, 정치인 비난 등"},
	{6, "종교관련", "종교 비하 또는 포교활동"},
	{7, "논쟁유발", "다툼, 논쟁, 분란을 조장"},
	{8, "허위사실", "거짓 정보 또는 루머 유포"},
	{9, "저작권위반", "저작물 무단 전재 또는 도용"},
	{10, "개인정보", "개인정보 무단 수집 또는 공개"},
	{11, "광고·홍보", "영리 목적 광고 또는 홍보"},
	{12, "음란물", "성인물, 음란 내용 게시"},
	{13, "불법정보", "불법 정보 또는 방법 공유"},
	{14, "사기행위", "사기, 피싱, 스캠 행위"},
	{15, "도배행위", "반복 게시, 스팸"},
	{16, "부정이용", "어뷰징, 시스템 악용"},
	{17, "다중이", "다중계정, 징계회피 재가입"},
	{18, "기타사유", "기타 전항 각호에 준하는 사유"},
}

// GetViolationType returns the violation type by code
func GetViolationType(code int) *ViolationType {
	for _, v := range ViolationTypes {
		if v.Code == code {
			return &v
		}
	}
	return nil
}

// DisciplineLogListResponse represents the response for discipline log list
type DisciplineLogListResponse struct {
	ID              uint64   `json:"id"`
	MemberID        string   `json:"member_id"`
	MemberNickname  string   `json:"member_nickname"`
	PenaltyPeriod   int      `json:"penalty_period"`
	PenaltyDateFrom string   `json:"penalty_date_from"`
	ViolationTypes  []int    `json:"violation_types"`
	ViolationTitles []string `json:"violation_titles"`
}

// DisciplineLogDetailResponse represents the response for discipline log detail
type DisciplineLogDetailResponse struct {
	ID              uint64           `json:"id"`
	MemberID        string           `json:"member_id"`
	MemberNickname  string           `json:"member_nickname"`
	PenaltyPeriod   int              `json:"penalty_period"`
	PenaltyDateFrom string           `json:"penalty_date_from"`
	PenaltyDateTo   *string          `json:"penalty_date_to,omitempty"`
	ViolationTypes  []ViolationType  `json:"violation_types"`
	ReportedItems   ReportedItemList `json:"reported_items,omitempty"`
	CreatedBy       string           `json:"created_by"`
	CreatedAt       string           `json:"created_at"`
	Status          string           `json:"status"`
}

// CreateDisciplineLogRequest represents the request for creating a discipline log
type CreateDisciplineLogRequest struct {
	MemberID        string         `json:"member_id" binding:"required"`
	MemberNickname  string         `json:"member_nickname" binding:"required"`
	PenaltyPeriod   int            `json:"penalty_period"` // -1: permanent, 0: warning, >0: days
	PenaltyDateFrom string         `json:"penalty_date_from" binding:"required"`
	ViolationTypes  []int          `json:"violation_types" binding:"required,min=1"`
	ReportedItems   []ReportedItem `json:"reported_items,omitempty"`
}

// ToListResponse converts DisciplineLog to list response
func (d *DisciplineLog) ToListResponse() DisciplineLogListResponse {
	titles := make([]string, 0, len(d.ViolationTypes))
	for _, code := range d.ViolationTypes {
		if vt := GetViolationType(code); vt != nil {
			titles = append(titles, vt.Title)
		}
	}

	return DisciplineLogListResponse{
		ID:              d.ID,
		MemberID:        d.MemberID,
		MemberNickname:  d.MemberNickname,
		PenaltyPeriod:   d.PenaltyPeriod,
		PenaltyDateFrom: d.PenaltyDateFrom.Format("2006-01-02"),
		ViolationTypes:  d.ViolationTypes,
		ViolationTitles: titles,
	}
}

// ToDetailResponse converts DisciplineLog to detail response
func (d *DisciplineLog) ToDetailResponse() DisciplineLogDetailResponse {
	violations := make([]ViolationType, 0, len(d.ViolationTypes))
	for _, code := range d.ViolationTypes {
		if vt := GetViolationType(code); vt != nil {
			violations = append(violations, *vt)
		}
	}

	resp := DisciplineLogDetailResponse{
		ID:              d.ID,
		MemberID:        d.MemberID,
		MemberNickname:  d.MemberNickname,
		PenaltyPeriod:   d.PenaltyPeriod,
		PenaltyDateFrom: d.PenaltyDateFrom.Format("2006-01-02 15:04:05"),
		ViolationTypes:  violations,
		ReportedItems:   d.ReportedItems,
		CreatedBy:       d.CreatedBy,
		CreatedAt:       d.CreatedAt.Format("2006-01-02 15:04:05"),
		Status:          d.Status,
	}

	if d.PenaltyDateTo != nil {
		to := d.PenaltyDateTo.Format("2006-01-02 15:04:05")
		resp.PenaltyDateTo = &to
	}

	return resp
}
