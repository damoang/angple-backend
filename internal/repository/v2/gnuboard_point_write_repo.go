package v2

import (
	"fmt"
	"time"

	"github.com/damoang/angple-backend/internal/domain/gnuboard"
	"gorm.io/gorm"
)

// ExpiringPointInfo represents a member with points expiring soon
type ExpiringPointInfo struct {
	MbID           string `json:"mb_id"`
	ExpiringAmount int    `json:"expiring_amount"`
}

// GnuboardPointWriteRepository handles g5_point writes + g5_member.mb_point balance management
type GnuboardPointWriteRepository interface {
	// AddPoint grants or deducts points with FIFO consumption
	AddPoint(mbID string, point int, content, relTable, relID, relAction string, pointConfig *PointConfig) error
	// CanAfford checks if the member has enough points
	CanAfford(mbID string, cost int) (bool, error)
	// ExpireBatch expires points past their expiry date (cron). Returns number of expired rows.
	ExpireBatch(batchSize int) (int, error)
	// GetExpiringPoints returns members with points expiring within N days
	GetExpiringPoints(withinDays int, limit int) ([]ExpiringPointInfo, error)
}

type gnuboardPointWriteRepository struct {
	db *gorm.DB
}

// NewGnuboardPointWriteRepository creates a new GnuboardPointWriteRepository
func NewGnuboardPointWriteRepository(db *gorm.DB) GnuboardPointWriteRepository {
	return &gnuboardPointWriteRepository{db: db}
}

// AddPoint grants or deducts points with FIFO consumption for deductions
func (r *gnuboardPointWriteRepository) AddPoint(mbID string, point int, content, relTable, relID, relAction string, pointConfig *PointConfig) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if point > 0 {
			return r.addPositivePoint(tx, mbID, point, content, relTable, relID, relAction, pointConfig)
		}
		return r.addNegativePoint(tx, mbID, point, content, relTable, relID, relAction)
	})
}

// addPositivePoint inserts a credit entry and updates member balance
func (r *gnuboardPointWriteRepository) addPositivePoint(tx *gorm.DB, mbID string, point int, content, relTable, relID, relAction string, pointConfig *PointConfig) error {
	// Determine expire date
	expireDate := "9999-12-31"
	if pointConfig != nil && pointConfig.ExpiryEnabled && pointConfig.ExpiryDays > 0 {
		expireDate = time.Now().AddDate(0, 0, pointConfig.ExpiryDays).Format("2006-01-02")
	}

	// Update member balance
	if err := tx.Table("g5_member").
		Where("mb_id = ?", mbID).
		UpdateColumn("mb_point", gorm.Expr("mb_point + ?", point)).Error; err != nil {
		return err
	}

	// Get updated balance for snapshot
	var mbPoint int
	if err := tx.Table("g5_member").Select("mb_point").Where("mb_id = ?", mbID).Scan(&mbPoint).Error; err != nil {
		return err
	}

	// Insert credit log
	entry := &gnuboard.G5Point{
		MbID:         mbID,
		PoDatetime:   time.Now(),
		PoContent:    content,
		PoPoint:      point,
		PoUsePoint:   0,
		PoExpired:    0,
		PoExpireDate: expireDate,
		PoRelTable:   relTable,
		PoRelID:      relID,
		PoRelAction:  relAction,
		MbPoint:      mbPoint,
	}
	return tx.Create(entry).Error
}

// addNegativePoint deducts points using FIFO consumption and updates member balance
func (r *gnuboardPointWriteRepository) addNegativePoint(tx *gorm.DB, mbID string, point int, content, relTable, relID, relAction string) error {
	absAmount := -point // positive value to consume

	// FIFO: find active credit entries ordered by expire date, then ID
	var credits []gnuboard.G5Point
	if err := tx.Raw(`
		SELECT po_id, po_point, po_use_point
		FROM g5_point
		WHERE po_mb_id = ? AND po_expired = 0 AND po_point > 0
		  AND (po_point - po_use_point) > 0
		ORDER BY po_expire_date ASC, po_id ASC
		FOR UPDATE
	`, mbID).Scan(&credits).Error; err != nil {
		return err
	}

	remaining := absAmount
	for _, credit := range credits {
		if remaining <= 0 {
			break
		}
		available := credit.PoPoint - credit.PoUsePoint
		consume := available
		if consume > remaining {
			consume = remaining
		}

		newUsePoint := credit.PoUsePoint + consume
		updates := map[string]interface{}{"po_use_point": newUsePoint}
		// If fully consumed, mark as expired (100 = consumed)
		if newUsePoint >= credit.PoPoint {
			updates["po_expired"] = 100
		}
		if err := tx.Table("g5_point").Where("po_id = ?", credit.PoID).Updates(updates).Error; err != nil {
			return err
		}
		remaining -= consume
	}

	// Update member balance
	if err := tx.Table("g5_member").
		Where("mb_id = ?", mbID).
		UpdateColumn("mb_point", gorm.Expr("mb_point + ?", point)).Error; err != nil {
		return err
	}

	// Get updated balance for snapshot
	var mbPoint int
	if err := tx.Table("g5_member").Select("mb_point").Where("mb_id = ?", mbID).Scan(&mbPoint).Error; err != nil {
		return err
	}

	// Insert deduction log
	entry := &gnuboard.G5Point{
		MbID:         mbID,
		PoDatetime:   time.Now(),
		PoContent:    content,
		PoPoint:      point,
		PoUsePoint:   0,
		PoExpired:    0,
		PoExpireDate: "9999-12-31",
		PoRelTable:   relTable,
		PoRelID:      relID,
		PoRelAction:  relAction,
		MbPoint:      mbPoint,
	}
	return tx.Create(entry).Error
}

// CanAfford checks if the member has enough points for a deduction
func (r *gnuboardPointWriteRepository) CanAfford(mbID string, cost int) (bool, error) {
	var mbPoint int
	if err := r.db.Table("g5_member").Select("mb_point").Where("mb_id = ?", mbID).Scan(&mbPoint).Error; err != nil {
		return false, err
	}
	// cost is negative for deductions
	return mbPoint+cost >= 0, nil
}

// ExpireBatch expires points past their expiry date in batches. Returns number of expired rows.
func (r *gnuboardPointWriteRepository) ExpireBatch(batchSize int) (int, error) {
	totalExpired := 0
	today := time.Now().Format("2006-01-02")

	for {
		var expired []struct {
			PoID       int    `gorm:"column:po_id"`
			MbID       string `gorm:"column:po_mb_id"`
			PoPoint    int    `gorm:"column:po_point"`
			PoUsePoint int    `gorm:"column:po_use_point"`
		}

		err := r.db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Raw(`
				SELECT po_id, po_mb_id, po_point, po_use_point
				FROM g5_point
				WHERE po_expired = 0 AND po_point > 0
				  AND po_expire_date < ?
				  AND po_expire_date != '9999-12-31'
				ORDER BY po_id ASC
				LIMIT ?
				FOR UPDATE
			`, today, batchSize).Scan(&expired).Error; err != nil {
				return err
			}

			if len(expired) == 0 {
				return nil
			}

			// Group by member for balance deduction
			memberDeductions := make(map[string]int)
			poIDs := make([]int, len(expired))
			for i, row := range expired {
				remaining := row.PoPoint - row.PoUsePoint
				if remaining > 0 {
					memberDeductions[row.MbID] += remaining
				}
				poIDs[i] = row.PoID
			}

			// Mark as expired (1 = time-expired)
			if err := tx.Table("g5_point").Where("po_id IN ?", poIDs).Update("po_expired", 1).Error; err != nil {
				return err
			}

			// Deduct remaining balance from each member
			for mbID, deduction := range memberDeductions {
				if err := tx.Table("g5_member").
					Where("mb_id = ?", mbID).
					UpdateColumn("mb_point", gorm.Expr("GREATEST(mb_point - ?, 0)", deduction)).Error; err != nil {
					return fmt.Errorf("deduct balance for %s: %w", mbID, err)
				}
			}

			return nil
		})

		if err != nil {
			return totalExpired, err
		}

		totalExpired += len(expired)

		// No more rows to process
		if len(expired) < batchSize {
			break
		}
	}

	return totalExpired, nil
}

// GetExpiringPoints returns members with points expiring within N days
func (r *gnuboardPointWriteRepository) GetExpiringPoints(withinDays int, limit int) ([]ExpiringPointInfo, error) {
	today := time.Now().Format("2006-01-02")
	futureDate := time.Now().AddDate(0, 0, withinDays).Format("2006-01-02")

	var results []ExpiringPointInfo
	err := r.db.Raw(`
		SELECT po_mb_id AS mb_id, SUM(po_point - po_use_point) AS expiring_amount
		FROM g5_point
		WHERE po_expired = 0 AND po_point > 0
		  AND po_expire_date BETWEEN ? AND ?
		  AND po_expire_date != '9999-12-31'
		  AND (po_point - po_use_point) > 0
		GROUP BY po_mb_id
		HAVING expiring_amount > 0
		LIMIT ?
	`, today, futureDate, limit).Scan(&results).Error

	return results, err
}
