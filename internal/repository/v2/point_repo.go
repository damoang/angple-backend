package v2

import (
	v2 "github.com/damoang/angple-backend/internal/domain/v2"
	"gorm.io/gorm"
)

// PointRepository v2 point transaction data access
type PointRepository interface {
	// CanAfford checks if user has enough points (for negative cost boards)
	CanAfford(userID uint64, cost int) (bool, error)
	// AddPoint atomically updates user point balance and logs the transaction
	AddPoint(userID uint64, point int, reason, relTable string, relID uint64) error
	// HasTransaction checks if a point transaction already exists for this relation
	HasTransaction(userID uint64, relTable string, relID uint64) (bool, error)
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
