package gnuboard

import (
	"fmt"
	"time"

	"github.com/damoang/angple-backend/internal/domain/gnuboard"
	"gorm.io/gorm"
)

// ScheduledDeleteRepository manages delayed deletion records
type ScheduledDeleteRepository interface {
	// Create inserts a new scheduled delete record
	Create(sd *gnuboard.ScheduledDelete) error
	// FindPending returns all pending records whose scheduled_at has passed
	FindPending(now time.Time, limit int) ([]gnuboard.ScheduledDelete, error)
	// FindByPost returns the pending scheduled delete for a specific post/comment
	FindByPost(boTable string, wrID int) (*gnuboard.ScheduledDelete, error)
	// Cancel marks a scheduled delete as cancelled
	Cancel(id int64) error
	// MarkExecuted marks a scheduled delete as executed
	MarkExecuted(id int64) error
	// DeleteByPost removes the scheduled delete for a post (cleanup after cancel)
	DeleteByPost(boTable string, wrID int) error
}

type scheduledDeleteRepository struct {
	db *gorm.DB
}

// NewScheduledDeleteRepository creates a new ScheduledDeleteRepository
func NewScheduledDeleteRepository(db *gorm.DB) ScheduledDeleteRepository {
	return &scheduledDeleteRepository{db: db}
}

func (r *scheduledDeleteRepository) Create(sd *gnuboard.ScheduledDelete) error {
	// 이전 cancelled/executed 레코드가 있으면 삭제하여 UNIQUE KEY 충돌 방지
	r.db.Where("bo_table = ? AND wr_id = ? AND status IN ('cancelled', 'executed')",
		sd.BoTable, sd.WrID).Delete(&gnuboard.ScheduledDelete{})
	return r.db.Create(sd).Error
}

func (r *scheduledDeleteRepository) FindPending(now time.Time, limit int) ([]gnuboard.ScheduledDelete, error) {
	var records []gnuboard.ScheduledDelete
	err := r.db.Where("status = 'pending' AND scheduled_at <= ?", now).
		Order("scheduled_at ASC").
		Limit(limit).
		Find(&records).Error
	return records, err
}

func (r *scheduledDeleteRepository) FindByPost(boTable string, wrID int) (*gnuboard.ScheduledDelete, error) {
	var sd gnuboard.ScheduledDelete
	err := r.db.Where("bo_table = ? AND wr_id = ? AND status = 'pending'", boTable, wrID).
		First(&sd).Error
	if err != nil {
		return nil, fmt.Errorf("scheduled delete not found: %w", err)
	}
	return &sd, nil
}

func (r *scheduledDeleteRepository) Cancel(id int64) error {
	now := time.Now()
	return r.db.Model(&gnuboard.ScheduledDelete{}).
		Where("id = ? AND status = 'pending'", id).
		Updates(map[string]interface{}{
			"status":       "cancelled",
			"cancelled_at": now,
		}).Error
}

func (r *scheduledDeleteRepository) MarkExecuted(id int64) error {
	now := time.Now()
	return r.db.Model(&gnuboard.ScheduledDelete{}).
		Where("id = ? AND status = 'pending'", id).
		Updates(map[string]interface{}{
			"status":      "executed",
			"executed_at": now,
		}).Error
}

func (r *scheduledDeleteRepository) DeleteByPost(boTable string, wrID int) error {
	return r.db.Where("bo_table = ? AND wr_id = ?", boTable, wrID).
		Delete(&gnuboard.ScheduledDelete{}).Error
}
