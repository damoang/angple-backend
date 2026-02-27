package v2

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// PointRepository point transaction data access (g5_point + g5_member 기반)
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

// NewPointRepository creates a new PointRepository (g5_point 기반)
func NewPointRepository(db *gorm.DB) PointRepository {
	return &pointRepository{db: db}
}

// resolveUserMbID v2_users.id → g5_member.mb_id (= v2_users.username)
func (r *pointRepository) resolveUserMbID(tx *gorm.DB, userID uint64) (string, error) {
	var username string
	err := tx.Table("v2_users").
		Select("username").
		Where("id = ?", userID).
		Scan(&username).Error
	if err != nil {
		return "", fmt.Errorf("사용자 조회 실패 (id=%d): %w", userID, err)
	}
	if username == "" {
		return "", fmt.Errorf("사용자 없음 (id=%d)", userID)
	}
	return username, nil
}

// deriveRelAction relTable에서 po_rel_action 도출 (PHP 호환)
func deriveRelAction(relTable string) string {
	switch relTable {
	case "v2_comments":
		return "comment"
	default:
		return "write"
	}
}

func (r *pointRepository) CanAfford(userID uint64, cost int) (bool, error) {
	mbID, err := r.resolveUserMbID(r.db, userID)
	if err != nil {
		return false, err
	}

	var mbPoint int
	if err := r.db.Raw("SELECT COALESCE(mb_point, 0) FROM g5_member WHERE mb_id = ?", mbID).
		Scan(&mbPoint).Error; err != nil {
		return false, err
	}
	// cost is negative for deductions, so user needs at least abs(cost)
	return mbPoint+cost >= 0, nil
}

func (r *pointRepository) AddPoint(userID uint64, point int, reason, relTable string, relID uint64) error {
	mbID, err := r.resolveUserMbID(r.db, userID)
	if err != nil {
		return err
	}

	return r.db.Transaction(func(tx *gorm.DB) error {
		// 현재 포인트 잔액 조회 (FOR UPDATE — 동시성 보호, PHP na_insert_point 호환)
		var mbPoint int
		if err := tx.Raw(
			"SELECT COALESCE(mb_point, 0) FROM g5_member WHERE mb_id = ? FOR UPDATE",
			mbID,
		).Scan(&mbPoint).Error; err != nil {
			return err
		}

		newBalance := mbPoint + point
		now := time.Now().Format("2006-01-02 15:04:05")

		// po_expired, po_expire_date (PHP na_insert_point 호환)
		poExpired := 0
		poExpireDate := "9999-12-31"
		if point < 0 {
			poExpired = 1
			poExpireDate = time.Now().Format("2006-01-02")
		}

		relAction := deriveRelAction(relTable)

		// g5_point INSERT (PHP na_insert_point 동일 형식)
		if err := tx.Exec(
			`INSERT INTO g5_point
				(mb_id, po_datetime, po_content, po_point, po_use_point, po_mb_point,
				 po_expired, po_expire_date, po_rel_table, po_rel_id, po_rel_action)
			VALUES (?, ?, ?, ?, 0, ?, ?, ?, ?, ?, ?)`,
			mbID, now, reason, point, newBalance,
			poExpired, poExpireDate, relTable, fmt.Sprintf("%d", relID), relAction,
		).Error; err != nil {
			return err
		}

		// g5_member.mb_point UPDATE
		return tx.Exec(
			"UPDATE g5_member SET mb_point = ? WHERE mb_id = ?",
			newBalance, mbID,
		).Error
	})
}

func (r *pointRepository) HasTransaction(userID uint64, relTable string, relID uint64) (bool, error) {
	mbID, err := r.resolveUserMbID(r.db, userID)
	if err != nil {
		return false, err
	}

	var count int64
	err = r.db.Table("g5_point").
		Where("mb_id = ? AND po_rel_table = ? AND po_rel_id = ?",
			mbID, relTable, fmt.Sprintf("%d", relID)).
		Count(&count).Error
	return count > 0, err
}
