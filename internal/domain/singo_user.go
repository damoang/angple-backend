package domain

// SingoUser — singo_users 테이블 (읽기 전용, ops-api에서 관리)
type SingoUser struct {
	ID   int    `gorm:"primaryKey"`
	MbID string `gorm:"column:mb_id"`
	Role string `gorm:"column:role"` // admin | super_admin
}

func (SingoUser) TableName() string {
	return "singo_users"
}
