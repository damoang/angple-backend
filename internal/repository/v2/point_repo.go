package v2

import (
	"time"

	v2 "github.com/damoang/angple-backend/internal/domain/v2"
	"gorm.io/gorm"
)

// PointSummary represents point summary statistics
type PointSummary struct {
	TotalPoint  int `json:"total_point"`
	TotalEarned int `json:"total_earned"`
	TotalUsed   int `json:"total_used"`
}

// PointHistory represents a point history item
type PointHistory struct {
	ID        uint64    `json:"id"`
	Point     int       `json:"point"`
	Balance   int       `json:"balance"`
	Reason    string    `json:"reason"`
	RelTable  string    `json:"rel_table"`
	RelID     uint64    `json:"rel_id"`
	CreatedAt time.Time `json:"created_at"`
}

// PointRepository v2 point transaction data access
type PointRepository interface {
	// CanAfford checks if user has enough points (for negative cost boards)
	CanAfford(userID uint64, cost int) (bool, error)
	// AddPoint atomically updates user point balance and logs the transaction
	AddPoint(userID uint64, point int, reason, relTable string, relID uint64) error
	// HasTransaction checks if a point transaction already exists for this relation
	HasTransaction(userID uint64, relTable string, relID uint64) (bool, error)
	// GetSummary returns point summary for a user
	GetSummary(userID uint64) (*PointSummary, error)
	// GetHistory returns point history with pagination
	GetHistory(userID uint64, filter string, page, limit int) ([]PointHistory, int64, error)
}

type pointRepository struct {
	db *gorm.DB
}

// NewPointRepository creates a new v2 PointRepository
func NewPointRepository(db *gorm.DB) PointRepository {
	return &pointRepository{db: db}
}

func (r *pointRepository) CanAfford(userID uint64, cost int) (bool, error) {
	var user v2.V2User
	if err := r.db.Select("point").Where("id = ?", userID).First(&user).Error; err != nil {
		return false, err
	}
	// cost is negative for deductions, so user needs at least abs(cost)
	return user.Point+cost >= 0, nil
}

func (r *pointRepository) AddPoint(userID uint64, point int, reason, relTable string, relID uint64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Update user point balance
		if err := tx.Model(&v2.V2User{}).
			Where("id = ?", userID).
			UpdateColumn("point", gorm.Expr("point + ?", point)).Error; err != nil {
			return err
		}

		// Get updated balance for log
		var user v2.V2User
		if err := tx.Select("point").Where("id = ?", userID).First(&user).Error; err != nil {
			return err
		}

		// Insert point log
		log := &v2.V2Point{
			UserID:   userID,
			Point:    point,
			Balance:  user.Point,
			Reason:   reason,
			RelTable: relTable,
			RelID:    relID,
		}
		return tx.Create(log).Error
	})
}

func (r *pointRepository) HasTransaction(userID uint64, relTable string, relID uint64) (bool, error) {
	var count int64
	err := r.db.Model(&v2.V2Point{}).
		Where("user_id = ? AND rel_table = ? AND rel_id = ?", userID, relTable, relID).
		Count(&count).Error
	return count > 0, err
}

func (r *pointRepository) GetSummary(userID uint64) (*PointSummary, error) {
	// Get current balance
	var user v2.V2User
	if err := r.db.Select("point").Where("id = ?", userID).First(&user).Error; err != nil {
		return nil, err
	}

	// Calculate total earned and used from history
	var totalEarned, totalUsed int

	// Sum positive points (earned)
	r.db.Model(&v2.V2Point{}).
		Select("COALESCE(SUM(point), 0)").
		Where("user_id = ? AND point > 0", userID).
		Scan(&totalEarned)

	// Sum negative points (used) - make it positive for display
	r.db.Model(&v2.V2Point{}).
		Select("COALESCE(ABS(SUM(point)), 0)").
		Where("user_id = ? AND point < 0", userID).
		Scan(&totalUsed)

	return &PointSummary{
		TotalPoint:  user.Point,
		TotalEarned: totalEarned,
		TotalUsed:   totalUsed,
	}, nil
}

func (r *pointRepository) GetHistory(userID uint64, filter string, page, limit int) ([]PointHistory, int64, error) {
	query := r.db.Model(&v2.V2Point{}).Where("user_id = ?", userID)

	// Apply filter
	switch filter {
	case "earned":
		query = query.Where("point > 0")
	case "used":
		query = query.Where("point < 0")
	}

	// Count total
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get paginated results
	offset := (page - 1) * limit
	var points []v2.V2Point
	if err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&points).Error; err != nil {
		return nil, 0, err
	}

	// Convert to PointHistory
	history := make([]PointHistory, len(points))
	for i, p := range points {
		history[i] = PointHistory{
			ID:        p.ID,
			Point:     p.Point,
			Balance:   p.Balance,
			Reason:    p.Reason,
			RelTable:  p.RelTable,
			RelID:     p.RelID,
			CreatedAt: p.CreatedAt,
		}
	}

	return history, total, nil
}
