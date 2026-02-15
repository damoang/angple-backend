package repository

import (
	"time"

	"gorm.io/gorm"
)

// SingoHistory represents a report status change history record
type SingoHistory struct {
	ID         int       `gorm:"primaryKey;autoIncrement"`
	Table      string    `gorm:"column:sg_table;size:50"`
	SGID       int       `gorm:"column:sg_id"`
	Parent     int       `gorm:"column:sg_parent"`
	PrevStatus string    `gorm:"column:prev_status;size:20"`
	NewStatus  string    `gorm:"column:new_status;size:20"`
	AdminID    string    `gorm:"column:admin_id;size:50"`
	Note       string    `gorm:"column:admin_note;type:text"`
	CreatedAt  time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (SingoHistory) TableName() string {
	return "g5_singo_history"
}

// HistoryRepository handles report history operations
type HistoryRepository struct {
	db *gorm.DB
}

// NewHistoryRepository creates a new HistoryRepository
func NewHistoryRepository(db *gorm.DB) *HistoryRepository {
	return &HistoryRepository{db: db}
}

// Record creates a history entry for a status transition
func (r *HistoryRepository) Record(table string, sgID, parent int, prevStatus, newStatus, adminID, note string) error {
	history := &SingoHistory{
		Table:      table,
		SGID:       sgID,
		Parent:     parent,
		PrevStatus: prevStatus,
		NewStatus:  newStatus,
		AdminID:    adminID,
		Note:       note,
	}
	return r.db.Create(history).Error
}

// GetByReport retrieves history for a specific report
func (r *HistoryRepository) GetByReport(table string, parent int) ([]SingoHistory, error) {
	var history []SingoHistory
	err := r.db.Where("sg_table = ? AND sg_parent = ?", table, parent).
		Order("created_at DESC").
		Find(&history).Error
	return history, err
}
