package domain

import (
	"strconv"
	"time"
)

// DisciplineLog represents a discipline log entry for g5_write_disciplinelog
type DisciplineLog struct {
	ID           int       `gorm:"column:wr_id;primaryKey;autoIncrement" json:"id"`
	Num          int       `gorm:"column:wr_num" json:"num"`
	Reply        string    `gorm:"column:wr_reply;size:10" json:"reply"`
	Parent       int       `gorm:"column:wr_parent" json:"parent"`
	IsComment    int8      `gorm:"column:wr_is_comment" json:"is_comment"`
	Comment      int       `gorm:"column:wr_comment" json:"comment"`
	CommentReply string    `gorm:"column:wr_comment_reply;size:5" json:"comment_reply"`
	CaName       string    `gorm:"column:ca_name;size:255" json:"ca_name"`
	Option       string    `gorm:"column:wr_option;size:40" json:"option"`
	Subject      string    `gorm:"column:wr_subject;size:255" json:"subject"`
	Content      string    `gorm:"column:wr_content;type:mediumtext" json:"content"`
	Link1        string    `gorm:"column:wr_link1;size:1000" json:"link1"`
	Link2        string    `gorm:"column:wr_link2;size:1000" json:"link2"`
	Link1Hit     int       `gorm:"column:wr_link1_hit" json:"link1_hit"`
	Link2Hit     int       `gorm:"column:wr_link2_hit" json:"link2_hit"`
	Hit          int       `gorm:"column:wr_hit" json:"hit"`
	Good         int       `gorm:"column:wr_good" json:"good"`
	Nogood       int       `gorm:"column:wr_nogood" json:"nogood"`
	MemberID     string    `gorm:"column:mb_id;size:20" json:"mb_id"`
	Name         string    `gorm:"column:wr_name;size:255" json:"name"`
	Password     string    `gorm:"column:wr_password;size:255" json:"password"`
	Email        string    `gorm:"column:wr_email;size:255" json:"email"`
	Homepage     string    `gorm:"column:wr_homepage;size:255" json:"homepage"`
	DateTime     time.Time `gorm:"column:wr_datetime" json:"datetime"`
	File         int8      `gorm:"column:wr_file" json:"file"`
	Last         string    `gorm:"column:wr_last;size:19" json:"last"`
	IP           string    `gorm:"column:wr_ip;size:255" json:"ip"`
	Facebook     string    `gorm:"column:wr_facebook_user;size:255" json:"facebook"`
	Twitter      string    `gorm:"column:wr_twitter_user;size:255" json:"twitter"`
	Wr1          string    `gorm:"column:wr_1;size:255" json:"wr_1"`
	Wr2          string    `gorm:"column:wr_2;size:255" json:"wr_2"`
	Wr3          string    `gorm:"column:wr_3;size:255" json:"wr_3"`
	Wr4          string    `gorm:"column:wr_4;size:255" json:"wr_4"` // 처리 상태: 'step2_approved'
	Wr5          string    `gorm:"column:wr_5;size:255" json:"wr_5"` // 원본 신고 테이블
	Wr6          string    `gorm:"column:wr_6;size:255" json:"wr_6"` // 원본 신고 ID
	Wr7          string    `gorm:"column:wr_7;size:255" json:"wr_7"` // 처리 유형
	Wr8          string    `gorm:"column:wr_8;size:255" json:"wr_8"`
	Wr9          string    `gorm:"column:wr_9;size:255" json:"wr_9"`
	Wr10         string    `gorm:"column:wr_10;size:255" json:"wr_10"`
}

// DisciplineLogContent represents the JSON content stored in discipline log
// PHP disciplinelog 뷰 스킨과 호환되는 필드명 사용
type DisciplineLogContent struct {
	// PHP 필수 필드 (disciplinelog 목록/상세 스킨에서 사용)
	PenaltyMbID     string         `json:"penalty_mb_id"`          // 피신고 회원 ID
	PenaltyDateFrom string         `json:"penalty_date_from"`      // 제재 시작일
	PenaltyPeriod   int            `json:"penalty_period"`         // 제한 일수 (0=주의, -1=영구)
	PenaltyType     []string       `json:"penalty_type"`           // ["level", "access"]
	SgTypes         []int          `json:"sg_types"`               // 사유 정수 코드 배열 (PHP 표시용)
	ReportedItems   []ReportedItem `json:"reported_items"`         // 신고된 글/댓글 목록
	ReportedURL     string         `json:"reported_url,omitempty"` // 단일 신고 URL (하위 호환)
	IsBulk          bool           `json:"is_bulk"`                // 일괄 처리 여부
	ReportCount     int            `json:"report_count"`           // 신고 건수
	Content         string         `json:"content,omitempty"`      // 상세 내용 (하위 호환)
	// Go 확장 필드 (PHP에서 무시됨, API 응답용)
	TargetNickname string   `json:"target_nickname,omitempty"`
	PenaltyReasons []string `json:"penalty_reasons,omitempty"` // 문자열 사유 코드 (Go API용)
	AdminMemo      string   `json:"admin_memo,omitempty"`
	ReportID       int      `json:"report_id"`
	ReportTable    string   `json:"report_table"`
	ProcessedBy    string   `json:"processed_by"`
}

// ReportedItem represents a reported post/comment in discipline log
type ReportedItem struct {
	Table  string `json:"table"`
	ID     int    `json:"id"`
	Parent int    `json:"parent"`
}

// ReasonKeyToCode maps string reason keys to integer codes (PHP SingoHelper 호환)
var ReasonKeyToCode = map[string]int{
	"member_insult":     1,
	"no_manner":         2,
	"inappropriate":     3,
	"discrimination":    4,
	"conflict":          5,
	"manipulation":      6,
	"deception":         7,
	"service_abuse":     8,
	"misuse":            9,
	"trading":           10,
	"begging":           11,
	"rights_violation":  12,
	"obscenity":         13,
	"illegal":           14,
	"advertising":       15,
	"policy_denial":     16,
	"multi_account":     17,
	"other":             18,
	"news_missing_info": 39,
	"news_full_text":    40,
}

// CodeToReasonKey는 정수 코드 → 텍스트 키 역매핑
var CodeToReasonKey = func() map[int]string {
	m := make(map[int]string, len(ReasonKeyToCode))
	for k, v := range ReasonKeyToCode {
		m[v] = k
	}
	return m
}()

// ResolveReasonCodes는 프론트엔드에서 전달된 사유 코드를 정수 배열로 변환한다.
// 텍스트 키("member_insult") 또는 숫자 문자열("1", "21") 모두 처리.
func ResolveReasonCodes(reasons []string) []int {
	codes := make([]int, 0, len(reasons))
	for _, r := range reasons {
		// 1차: 텍스트 키 매핑 (기존 방식)
		if code, ok := ReasonKeyToCode[r]; ok {
			codes = append(codes, code)
			continue
		}
		// 2차: 숫자 문자열 직접 파싱 ("1", "21" 등)
		if num, err := strconv.Atoi(r); err == nil && num > 0 {
			codes = append(codes, num)
		}
	}
	return codes
}

// DisciplineResponse represents a discipline log entry for API response
type DisciplineResponse struct {
	ID           int                   `json:"id"`
	Subject      string                `json:"subject"`
	Status       string                `json:"status"`       // wr_4
	ProcessType  string                `json:"process_type"` // wr_7
	Content      *DisciplineLogContent `json:"content,omitempty"`
	CreatedAt    string                `json:"created_at"`
	CommentCount int                   `json:"comment_count"`
}

// AppealRequest represents a request to submit an appeal
type AppealRequest struct {
	Content string `json:"content" binding:"required"`
}

// G5Memo represents a message (쪽지) in g5_memo table
type G5Memo struct {
	ID           int       `gorm:"column:me_id;primaryKey;autoIncrement" json:"id"`
	RecvMemberID string    `gorm:"column:me_recv_mb_id;size:20;index" json:"recv_mb_id"`
	SendMemberID string    `gorm:"column:me_send_mb_id;size:20;index" json:"send_mb_id"`
	SendDatetime time.Time `gorm:"column:me_send_datetime" json:"send_datetime"`
	ReadDatetime string    `gorm:"column:me_read_datetime;size:19" json:"read_datetime"`
	Memo         string    `gorm:"column:me_memo;type:text" json:"memo"`
	SendID       int       `gorm:"column:me_send_id" json:"send_id"`   // 원본 쪽지 ID (발신함)
	Type         string    `gorm:"column:me_type;size:10" json:"type"` // 'recv' or 'send'
	SendIP       string    `gorm:"column:me_send_ip;size:100" json:"send_ip,omitempty"`
}

// TableName returns the table name for G5Memo
func (G5Memo) TableName() string {
	return "g5_memo"
}
