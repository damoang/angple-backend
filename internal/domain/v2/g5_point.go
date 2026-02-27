package v2

// G5Point represents a row in the g5_point table (PHP 그누보드 포인트 내역)
type G5Point struct {
	PoID        int    `gorm:"column:po_id;primaryKey;autoIncrement" json:"id"`
	MbID        string `gorm:"column:mb_id" json:"mb_id"`
	PoDatetime  string `gorm:"column:po_datetime" json:"po_datetime"`
	PoContent   string `gorm:"column:po_content" json:"po_content"`
	PoPoint     int    `gorm:"column:po_point" json:"po_point"`
	PoUsePoint  int    `gorm:"column:po_use_point" json:"po_use_point"`
	PoExpired   int8   `gorm:"column:po_expired" json:"po_expired"`
	PoExpireDate string `gorm:"column:po_expire_date" json:"po_expire_date"`
	PoMbPoint   int    `gorm:"column:po_mb_point" json:"po_mb_point"`
	PoRelTable  string `gorm:"column:po_rel_table" json:"po_rel_table"`
	PoRelID     string `gorm:"column:po_rel_id" json:"po_rel_id"`
	PoRelAction string `gorm:"column:po_rel_action" json:"po_rel_action"`
}

func (G5Point) TableName() string { return "g5_point" }
