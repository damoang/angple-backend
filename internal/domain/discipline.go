package domain

import "time"

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
	Facebook     string    `gorm:"column:wr_facebook;size:255" json:"facebook"`
	Twitter      string    `gorm:"column:wr_twitter;size:255" json:"twitter"`
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
type DisciplineLogContent struct {
	TargetID       string   `json:"target_id"`
	TargetNickname string   `json:"target_nickname,omitempty"`
	PenaltyDays    int      `json:"penalty_days"`    // 제한 일수 (0=주의, -1=영구)
	PenaltyType    []string `json:"penalty_type"`    // ["level", "intercept"]
	PenaltyReasons []string `json:"penalty_reasons"` // 신고 사유 코드
	AdminMemo      string   `json:"admin_memo,omitempty"`
	ReportID       int      `json:"report_id"`
	ReportTable    string   `json:"report_table"`
	TargetContent  string   `json:"target_content,omitempty"`
	TargetTitle    string   `json:"target_title,omitempty"`
	ProcessedAt    string   `json:"processed_at"`
	ProcessedBy    string   `json:"processed_by"`
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
