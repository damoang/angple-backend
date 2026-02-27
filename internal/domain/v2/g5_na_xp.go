package v2

// G5NaXp represents a row in the g5_na_xp table (나리야 경험치 내역)
type G5NaXp struct {
	XpID        int    `gorm:"column:xp_id;primaryKey;autoIncrement" json:"id"`
	MbID        string `gorm:"column:mb_id" json:"mb_id"`
	XpDatetime  string `gorm:"column:xp_datetime" json:"exp_datetime"`
	XpContent   string `gorm:"column:xp_content" json:"exp_content"`
	XpPoint     int    `gorm:"column:xp_point" json:"exp_point"`
	XpRelTable  string `gorm:"column:xp_rel_table" json:"exp_rel_table"`
	XpRelID     string `gorm:"column:xp_rel_id" json:"exp_rel_id"`
	XpRelAction string `gorm:"column:xp_rel_action" json:"exp_rel_action"`
}

func (G5NaXp) TableName() string { return "g5_na_xp" }
