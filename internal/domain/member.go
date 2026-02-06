package domain

import (
	"time"
)

// Member domain model (g5_member table)
type Member struct {
	TodayLogin    time.Time `gorm:"column:mb_today_login" json:"-"`
	OpenDate      time.Time `gorm:"column:mb_open_date" json:"-"`
	EmailCertify  time.Time `gorm:"column:mb_email_certify" json:"-"`
	CreatedAt     time.Time `gorm:"column:mb_datetime" json:"created_at"`
	Birth         string    `gorm:"column:mb_birth" json:"birth,omitempty"`
	Memo          string    `gorm:"column:mb_memo" json:"-"`
	Homepage      string    `gorm:"column:mb_homepage" json:"homepage,omitempty"`
	MemoCall      string    `gorm:"column:mb_memo_call" json:"-"`
	Profile       string    `gorm:"column:mb_profile" json:"profile,omitempty"`
	Sex           string    `gorm:"column:mb_sex" json:"sex,omitempty"`
	UserID        string    `gorm:"column:mb_id;uniqueIndex" json:"user_id"`
	Tel           string    `gorm:"column:mb_tel" json:"-"`
	Phone         string    `gorm:"column:mb_hp" json:"-"`
	Nickname      string    `gorm:"column:mb_nick" json:"nickname"`
	LostCertify   string    `gorm:"column:mb_lost_certify" json:"-"`
	DupInfo       string    `gorm:"column:mb_dupinfo" json:"-"`
	Zip1          string    `gorm:"column:mb_zip1" json:"-"`
	Zip2          string    `gorm:"column:mb_zip2" json:"-"`
	Addr1         string    `gorm:"column:mb_addr1" json:"-"`
	Addr2         string    `gorm:"column:mb_addr2" json:"-"`
	Addr3         string    `gorm:"column:mb_addr3" json:"-"`
	AddrJibeon    string    `gorm:"column:mb_addr_jibeon" json:"-"`
	Email         string    `gorm:"column:mb_email;uniqueIndex" json:"email"`
	Recommend     string    `gorm:"column:mb_recommend" json:"-"`
	Certify       string    `gorm:"column:mb_certify" json:"-"`
	LoginIP       string    `gorm:"column:mb_login_ip" json:"-"`
	IP            string    `gorm:"column:mb_ip" json:"-"`
	Name          string    `gorm:"column:mb_name" json:"name"`
	LeaveDate     string    `gorm:"column:mb_leave_date" json:"-"`
	InterceptDate string    `gorm:"column:mb_intercept_date" json:"-"`
	Password      string    `gorm:"column:mb_password" json:"-"`
	EmailCertify2 string    `gorm:"column:mb_email_certify2" json:"-"`
	Signature     string    `gorm:"column:mb_signature" json:"-"`
	Adult         int       `gorm:"column:mb_adult" json:"-"`
	MailingNormal int       `gorm:"column:mb_mailling_normal" json:"-"`
	MailingSms    int       `gorm:"column:mb_mailling_sms" json:"-"`
	Open          int       `gorm:"column:mb_open" json:"-"`
	ID            int       `gorm:"column:mb_no;primaryKey" json:"id"`
	Point         int       `gorm:"column:mb_point" json:"point"`
	Level         int       `gorm:"column:mb_level" json:"level"`
	MemoCount     int       `gorm:"column:mb_memo_cnt" json:"-"`
	ScrapCount    int       `gorm:"column:mb_scrap_cnt" json:"-"`
	AsExp         int       `gorm:"column:as_exp" json:"as_exp"`
	AsLevel       int       `gorm:"column:as_level" json:"as_level"`
	AsMax         int       `gorm:"column:as_max" json:"as_max"`
}

func (Member) TableName() string {
	return "g5_member"
}

type MemberResponse struct {
	UserID   string `json:"user_id"`
	Name     string `json:"name"`
	Nickname string `json:"nickname"`
	Email    string `json:"email"`
	Profile  string `json:"profile,omitempty"`
	ID       int    `json:"id"`
	Level    int    `json:"level"`
	Point    int    `json:"point"`
}

func (m *Member) ToResponse() *MemberResponse {
	return &MemberResponse{
		ID:       m.ID,
		UserID:   m.UserID,
		Name:     m.Name,
		Nickname: m.Nickname,
		Email:    m.Email,
		Level:    m.Level,
		Point:    m.Point,
		Profile:  m.Profile,
	}
}

// MemberProfileResponse represents a public member profile
type MemberProfileResponse struct {
	UserID    string `json:"user_id"`
	Nickname  string `json:"nickname"`
	Profile   string `json:"profile,omitempty"`
	CreatedAt string `json:"created_at"`
	Level     int    `json:"level"`
	Point     int    `json:"point"`
}

// ToProfileResponse converts Member to MemberProfileResponse
func (m *Member) ToProfileResponse() *MemberProfileResponse {
	return &MemberProfileResponse{
		UserID:    m.UserID,
		Nickname:  m.Nickname,
		Profile:   m.Profile,
		Level:     m.Level,
		Point:     m.Point,
		CreatedAt: m.CreatedAt.Format("2006-01-02"),
	}
}

// MemberPostSummary represents a summary of a member's post
type MemberPostSummary struct {
	CreatedAt string `json:"created_at"`
	BoardID   string `json:"board_id"`
	Title     string `json:"title"`
	ID        int    `json:"id"`
	Comments  int    `json:"comments_count"`
	Likes     int    `json:"likes"`
	Views     int    `json:"views"`
}

// MemberCommentSummary represents a summary of a member's comment
type MemberCommentSummary struct {
	CreatedAt string `json:"created_at"`
	BoardID   string `json:"board_id"`
	Content   string `json:"content"`
	ID        int    `json:"id"`
	PostID    int    `json:"post_id"`
}

// PointHistory represents a point transaction record (g5_point table)
type PointHistory struct {
	CreatedAt string `json:"created_at"`
	Content   string `json:"content"`
	RelTable  string `json:"rel_table,omitempty"`
	RelAction string `json:"rel_action,omitempty"`
	ID        int    `json:"id"`
	Point     int    `json:"point"`
	RelID     int    `json:"rel_id,omitempty"`
}

// Point domain model (g5_point table)
type Point struct {
	Datetime   string `gorm:"column:po_datetime" json:"datetime"`
	Content    string `gorm:"column:po_content" json:"content"`
	RelTable   string `gorm:"column:po_rel_table" json:"rel_table"`
	RelAction  string `gorm:"column:po_rel_action" json:"rel_action"`
	MbID       string `gorm:"column:mb_id" json:"mb_id"`
	RelID      string `gorm:"column:po_rel_id" json:"rel_id"`
	ID         int    `gorm:"column:po_id;primaryKey" json:"id"`
	UsePoint   int    `gorm:"column:po_use_point" json:"use_point"`
	Point      int    `gorm:"column:po_point" json:"point"`
	MbPoint    int    `gorm:"column:po_mb_point" json:"mb_point"`
	ExpireDate string `gorm:"column:po_expire_date" json:"expire_date"`
}

func (Point) TableName() string {
	return "g5_point"
}
