package domain

// SingoSetting — singo_settings 키-값 설정 테이블
type SingoSetting struct {
	Key       string `gorm:"column:key;primaryKey" json:"key"`
	Value     string `gorm:"column:value" json:"value"`
	UpdatedBy string `gorm:"column:updated_by" json:"updated_by"`
}

func (SingoSetting) TableName() string {
	return "singo_settings"
}
