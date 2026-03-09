package v2

import (
	"github.com/damoang/angple-backend/internal/domain/gnuboard"
	"gorm.io/gorm"
)

// GnuboardPointRepository handles g5_point and g5_member point data access
type GnuboardPointRepository interface {
	// GetSummary returns point summary for a user (by mb_id)
	GetSummary(mbID string) (*PointSummary, error)
	// GetHistory returns point history with pagination and optional filter
	GetHistory(mbID string, filter string, page, limit int) ([]gnuboard.PointHistoryItem, int64, error)
}

type gnuboardPointRepository struct {
	db *gorm.DB
}

// NewGnuboardPointRepository creates a new GnuboardPointRepository
func NewGnuboardPointRepository(db *gorm.DB) GnuboardPointRepository {
	return &gnuboardPointRepository{db: db}
}

func (r *gnuboardPointRepository) GetSummary(mbID string) (*PointSummary, error) {
	// Get current balance from g5_member
	var member gnuboard.G5Member
	if err := r.db.Select("mb_point").Where("mb_id = ?", mbID).First(&member).Error; err != nil {
		return nil, err
	}

	// Calculate total earned and used in a single query
	var result struct {
		TotalEarned int
		TotalUsed   int
	}
	r.db.Model(&gnuboard.G5Point{}).
		Select("COALESCE(SUM(CASE WHEN po_point > 0 THEN po_point ELSE 0 END), 0) as total_earned, COALESCE(SUM(CASE WHEN po_point < 0 THEN ABS(po_point) ELSE 0 END), 0) as total_used").
		Where("po_mb_id = ?", mbID).
		Scan(&result)

	return &PointSummary{
		TotalPoint:  member.MbPoint,
		TotalEarned: result.TotalEarned,
		TotalUsed:   result.TotalUsed,
	}, nil
}

func (r *gnuboardPointRepository) GetHistory(mbID string, filter string, page, limit int) ([]gnuboard.PointHistoryItem, int64, error) {
	query := r.db.Model(&gnuboard.G5Point{}).Where("po_mb_id = ?", mbID)

	// Apply filter
	switch filter {
	case "earned":
		query = query.Where("po_point > 0")
	case "used":
		query = query.Where("po_point < 0")
	}

	// Count total
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get paginated results
	offset := (page - 1) * limit
	var points []gnuboard.G5Point
	if err := query.Order("po_id DESC").Offset(offset).Limit(limit).Find(&points).Error; err != nil {
		return nil, 0, err
	}

	// Convert to PointHistoryItem
	items := make([]gnuboard.PointHistoryItem, len(points))
	for i, p := range points {
		items[i] = p.ToHistoryItem()
	}

	return items, total, nil
}
