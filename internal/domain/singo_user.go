package domain

// SingoUser — singo_users 테이블
type SingoUser struct {
	ID   int    `gorm:"primaryKey"`
	MbID string `gorm:"column:mb_id"`
	Role string `gorm:"column:role"` // admin | super_admin
}

func (SingoUser) TableName() string {
	return "singo_users"
}

// SingoUserWithNick — singo_users + g5_member.mb_nick JOIN 결과
type SingoUserWithNick struct {
	ID     int    `json:"id"`
	MbID   string `json:"mb_id"`
	Role   string `json:"role"`
	MbNick string `json:"mb_nick"`
}
