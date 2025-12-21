package domain

import (
	"time"
)

// Member domain model (g5_member table)
type Member struct {
	ID            int       `gorm:"column:mb_no;primaryKey" json:"id"`
	UserID        string    `gorm:"column:mb_id;uniqueIndex" json:"user_id"`
	Password      string    `gorm:"column:mb_password" json:"-"`
	Name          string    `gorm:"column:mb_name" json:"name"`
	Nickname      string    `gorm:"column:mb_nick" json:"nickname"`
	Email         string    `gorm:"column:mb_email;uniqueIndex" json:"email"`
	Homepage      string    `gorm:"column:mb_homepage" json:"homepage,omitempty"`
	Level         int       `gorm:"column:mb_level" json:"level"`
	Point         int       `gorm:"column:mb_point" json:"point"`
	Sex           string    `gorm:"column:mb_sex" json:"sex,omitempty"`
	Birth         string    `gorm:"column:mb_birth" json:"birth,omitempty"`
	Tel           string    `gorm:"column:mb_tel" json:"-"`
	Phone         string    `gorm:"column:mb_hp" json:"-"`
	Certify       string    `gorm:"column:mb_certify" json:"-"`
	Adult         int       `gorm:"column:mb_adult" json:"-"`
	DupInfo       string    `gorm:"column:mb_dupinfo" json:"-"`
	Zip1          string    `gorm:"column:mb_zip1" json:"-"`
	Zip2          string    `gorm:"column:mb_zip2" json:"-"`
	Addr1         string    `gorm:"column:mb_addr1" json:"-"`
	Addr2         string    `gorm:"column:mb_addr2" json:"-"`
	Addr3         string    `gorm:"column:mb_addr3" json:"-"`
	AddrJibeon    string    `gorm:"column:mb_addr_jibeon" json:"-"`
	Signature     string    `gorm:"column:mb_signature" json:"-"`
	Recommend     string    `gorm:"column:mb_recommend" json:"-"`
	TodayLogin    time.Time `gorm:"column:mb_today_login" json:"-"`
	LoginIP       string    `gorm:"column:mb_login_ip" json:"-"`
	IP            string    `gorm:"column:mb_ip" json:"-"`
	CreatedAt     time.Time `gorm:"column:mb_datetime" json:"created_at"`
	LeaveDate     string    `gorm:"column:mb_leave_date" json:"-"`
	InterceptDate string    `gorm:"column:mb_intercept_date" json:"-"`
	EmailCertify  time.Time `gorm:"column:mb_email_certify" json:"-"`
	EmailCertify2 string    `gorm:"column:mb_email_certify2" json:"-"`
	Memo          string    `gorm:"column:mb_memo" json:"-"`
	LostCertify   string    `gorm:"column:mb_lost_certify" json:"-"`
	MailingNormal int       `gorm:"column:mb_mailling_normal" json:"-"`
	MailingSms    int       `gorm:"column:mb_mailling_sms" json:"-"`
	Open          int       `gorm:"column:mb_open" json:"-"`
	OpenDate      time.Time `gorm:"column:mb_open_date" json:"-"`
	Profile       string    `gorm:"column:mb_profile" json:"profile,omitempty"`
	MemoCall      string    `gorm:"column:mb_memo_call" json:"-"`
	MemoCount     int       `gorm:"column:mb_memo_cnt" json:"-"`
	ScrapCount    int       `gorm:"column:mb_scrap_cnt" json:"-"`
}

func (Member) TableName() string {
	return "g5_member"
}

type MemberResponse struct {
	ID       int    `json:"id"`
	UserID   string `json:"user_id"`
	Name     string `json:"name"`
	Nickname string `json:"nickname"`
	Email    string `json:"email"`
	Level    int    `json:"level"`
	Point    int    `json:"point"`
	Profile  string `json:"profile,omitempty"`
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
