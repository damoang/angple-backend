package repository

import (
	"github.com/damoang/angple-backend/internal/domain"
	"gorm.io/gorm"
)

type ContentHistoryRepository struct {
	db *gorm.DB
}

func NewContentHistoryRepository(db *gorm.DB) *ContentHistoryRepository {
	return &ContentHistoryRepository{db: db}
}

// FindByTableAndID returns content history records for a specific board table and write ID
func (r *ContentHistoryRepository) FindByTableAndID(boTable string, wrID int) ([]domain.ContentHistory, error) {
	var records []domain.ContentHistory
	err := r.db.Where("bo_table = ? AND wr_id = ?", boTable, wrID).
		Order("operated_at DESC").
		Limit(20).
		Find(&records).Error
	if err != nil {
		return nil, err
	}
	return records, nil
}
