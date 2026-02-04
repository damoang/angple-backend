package repository

import (
	"github.com/damoang/angple-backend/internal/domain"
	"gorm.io/gorm"
)

// PointRepository point data access interface
type PointRepository interface {
	FindByMemberID(mbID string, limit int) ([]*domain.Point, error)
}

type pointRepository struct {
	db *gorm.DB
}

// NewPointRepository creates a new PointRepository
func NewPointRepository(db *gorm.DB) PointRepository {
	return &pointRepository{db: db}
}

// FindByMemberID returns recent point history for a member
func (r *pointRepository) FindByMemberID(mbID string, limit int) ([]*domain.Point, error) {
	var points []*domain.Point
	err := r.db.Where("mb_id = ?", mbID).
		Order("po_id DESC").
		Limit(limit).
		Find(&points).Error
	if err != nil {
		return nil, err
	}
	return points, nil
}
