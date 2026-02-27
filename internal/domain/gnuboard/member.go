package gnuboard

import "time"

// G5Member represents the g5_member table (Gnuboard member/user)
type G5Member struct {
	MbNo            int       `gorm:"column:mb_no;primaryKey;autoIncrement" json:"mb_no"`
	MbID            string    `gorm:"column:mb_id;uniqueIndex" json:"mb_id"`
	MbPassword      string    `gorm:"column:mb_password" json:"-"`
	MbName          string    `gorm:"column:mb_name" json:"mb_name"`
	MbNick          string    `gorm:"column:mb_nick" json:"mb_nick"`
	MbNickDate      string    `gorm:"column:mb_nick_date" json:"mb_nick_date"`
	MbEmail         string    `gorm:"column:mb_email" json:"mb_email"`
	MbHomepage      string    `gorm:"column:mb_homepage" json:"mb_homepage"`
	MbLevel         int       `gorm:"column:mb_level" json:"mb_level"`
	MbSex           string    `gorm:"column:mb_sex" json:"mb_sex"`
	MbBirth         string    `gorm:"column:mb_birth" json:"mb_birth"`
	MbTel           string    `gorm:"column:mb_tel" json:"mb_tel"`
	MbHp            string    `gorm:"column:mb_hp" json:"mb_hp"`
	MbCertify       string    `gorm:"column:mb_certify" json:"mb_certify"`
	MbAdult         int       `gorm:"column:mb_adult" json:"mb_adult"`
	MbDupInfo       string    `gorm:"column:mb_dupinfo" json:"mb_dupinfo"`
	MbZip1          string    `gorm:"column:mb_zip1" json:"mb_zip1"`
	MbZip2          string    `gorm:"column:mb_zip2" json:"mb_zip2"`
	MbAddr1         string    `gorm:"column:mb_addr1" json:"mb_addr1"`
	MbAddr2         string    `gorm:"column:mb_addr2" json:"mb_addr2"`
	MbAddr3         string    `gorm:"column:mb_addr3" json:"mb_addr3"`
	MbAddrJibeon    string    `gorm:"column:mb_addr_jibeon" json:"mb_addr_jibeon"`
	MbSignature     string    `gorm:"column:mb_signature" json:"mb_signature"`
	MbRecommend     string    `gorm:"column:mb_recommend" json:"mb_recommend"`
	MbPoint         int       `gorm:"column:mb_point" json:"mb_point"`
	MbTodayLogin    string    `gorm:"column:mb_today_login" json:"mb_today_login"`
	MbLoginIP       string    `gorm:"column:mb_login_ip" json:"-"`
	MbDatetime      time.Time `gorm:"column:mb_datetime" json:"mb_datetime"`
	MbIP            string    `gorm:"column:mb_ip" json:"-"`
	MbLeaveDate     string    `gorm:"column:mb_leave_date" json:"mb_leave_date"`
	MbInterceptDate string    `gorm:"column:mb_intercept_date" json:"mb_intercept_date"`
	MbEmailCertify  string    `gorm:"column:mb_email_certify" json:"mb_email_certify"`
	MbMemo          string    `gorm:"column:mb_memo" json:"-"`
	MbLost          string    `gorm:"column:mb_lost_certify" json:"mb_lost_certify"`
	MbMailling      int       `gorm:"column:mb_mailling" json:"mb_mailling"`
	MbSms           int       `gorm:"column:mb_sms" json:"mb_sms"`
	MbOpen          int       `gorm:"column:mb_open" json:"mb_open"`
	MbOpenDate      string    `gorm:"column:mb_open_date" json:"mb_open_date"`
	MbProfile       string    `gorm:"column:mb_profile" json:"mb_profile"`
	MbMemoCall      string    `gorm:"column:mb_memo_call" json:"mb_memo_call"`
	MbMemoCallN     int       `gorm:"column:mb_memo_cnt" json:"mb_memo_cnt"`
	MbScrap         int       `gorm:"column:mb_scrap_cnt" json:"mb_scrap_cnt"`
	Mb1             string    `gorm:"column:mb_1" json:"mb_1"`
	Mb2             string    `gorm:"column:mb_2" json:"mb_2"`
	Mb3             string    `gorm:"column:mb_3" json:"mb_3"`
	Mb4             string    `gorm:"column:mb_4" json:"mb_4"`
	Mb5             string    `gorm:"column:mb_5" json:"mb_5"`
	Mb6             string    `gorm:"column:mb_6" json:"mb_6"`
	Mb7             string    `gorm:"column:mb_7" json:"mb_7"`
	Mb8             string    `gorm:"column:mb_8" json:"mb_8"`
	Mb9             string    `gorm:"column:mb_9" json:"mb_9"`
	Mb10            string    `gorm:"column:mb_10" json:"mb_10"`
	MbIconPath      string    `gorm:"column:mb_icon_path" json:"mb_icon_path"`
	MbImagePath     string    `gorm:"column:mb_image_path" json:"mb_image_path"`
}

// TableName returns the table name for GORM
func (G5Member) TableName() string {
	return "g5_member"
}

// IsActive checks if the member is active (not left or banned)
func (m *G5Member) IsActive() bool {
	return m.MbLeaveDate == "" && m.MbInterceptDate == ""
}

// MemberResponse is the API response format for member
type MemberResponse struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Nickname  string    `json:"nickname"`
	Email     string    `json:"email"`
	Level     int       `json:"level"`
	Point     int       `json:"point"`
	AvatarURL string    `json:"avatar_url,omitempty"`
	Profile   string    `json:"profile,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// ToResponse converts G5Member to API response format
func (m *G5Member) ToResponse() MemberResponse {
	return MemberResponse{
		ID:        m.MbID,
		Username:  m.MbID,
		Nickname:  m.MbNick,
		Email:     m.MbEmail,
		Level:     m.MbLevel,
		Point:     m.MbPoint,
		AvatarURL: m.MbIconPath,
		Profile:   m.MbProfile,
		CreatedAt: m.MbDatetime,
	}
}
