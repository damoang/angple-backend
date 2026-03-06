package gnuboard

import "time"

// G5Point represents the g5_point table (Gnuboard point log)
type G5Point struct {
	PoID         int       `gorm:"column:po_id;primaryKey;autoIncrement" json:"po_id"`
	MbID         string    `gorm:"column:po_mb_id;index" json:"po_mb_id"`
	PoDatetime   time.Time `gorm:"column:po_datetime" json:"po_datetime"`
	PoContent    string    `gorm:"column:po_content" json:"po_content"`
	PoPoint      int       `gorm:"column:po_point" json:"po_point"`
	PoUsePoint   int       `gorm:"column:po_use_point" json:"po_use_point"`
	PoExpired    int       `gorm:"column:po_expired" json:"po_expired"`
	PoExpireDate string    `gorm:"column:po_expire_date" json:"po_expire_date"`
	PoRelTable   string    `gorm:"column:po_rel_table" json:"po_rel_table"`
	PoRelID      string    `gorm:"column:po_rel_id" json:"po_rel_id"`
	PoRelAction  string    `gorm:"column:po_rel_action" json:"po_rel_action"`
	MbPoint      int       `gorm:"column:mb_point" json:"mb_point"`
}

// TableName returns the table name for GORM
func (G5Point) TableName() string {
	return "g5_point"
}

// PointHistoryItem represents a point history item for API response
type PointHistoryItem struct {
	ID           int    `json:"id"`
	MbID         string `json:"mb_id"`
	PoContent    string `json:"po_content"`
	PoPoint      int    `json:"po_point"`
	PoDatetime   string `json:"po_datetime"`
	PoExpired    int    `json:"po_expired"`
	PoExpireDate string `json:"po_expire_date"`
	PoRelTable   string `json:"po_rel_table,omitempty"`
	PoRelID      string `json:"po_rel_id,omitempty"`
	PoRelAction  string `json:"po_rel_action,omitempty"`
}

// ToHistoryItem converts G5Point to PointHistoryItem
func (p *G5Point) ToHistoryItem() PointHistoryItem {
	return PointHistoryItem{
		ID:           p.PoID,
		MbID:         p.MbID,
		PoContent:    p.PoContent,
		PoPoint:      p.PoPoint,
		PoDatetime:   p.PoDatetime.Format(time.RFC3339),
		PoExpired:    p.PoExpired,
		PoExpireDate: p.PoExpireDate,
		PoRelTable:   p.PoRelTable,
		PoRelID:      p.PoRelID,
		PoRelAction:  p.PoRelAction,
	}
}
