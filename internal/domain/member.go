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
