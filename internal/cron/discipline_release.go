package cron

import (
	"log"
	"time"

	"gorm.io/gorm"
)

// DisciplineReleaseResult contains the result of discipline release
type DisciplineReleaseResult struct {
	LevelRestoredCount    int      `json:"level_restored_count"`
	LevelRestoredIDs      []string `json:"level_restored_ids"`
	InterceptReleasedCount int     `json:"intercept_released_count"`
	InterceptReleasedIDs   []string `json:"intercept_released_ids"`
	ExecutedAt            string   `json:"executed_at"`
}

// runDisciplineRelease restores level and clears intercept for expired discipline records
func runDisciplineRelease(db *gorm.DB) (*DisciplineReleaseResult, error) {
	now := time.Now()
	result := &DisciplineReleaseResult{
		ExecutedAt: now.Format("2006-01-02 15:04:05"),
	}

	// 1. Restore levels for expired level-type disciplines
	// Conditions: penalty_type includes 'level' or 'all', period > 0 (not permanent), expired, member still at level 1
	type expiredDiscipline struct {
		PenaltyMbID string `gorm:"column:penalty_mb_id"`
		PrevLevel   int    `gorm:"column:prev_level"`
	}

	var expired []expiredDiscipline
	if err := db.Raw(`
		SELECT d.penalty_mb_id, d.prev_level
		FROM g5_da_member_discipline d
		JOIN g5_member m ON m.mb_id = d.penalty_mb_id
		WHERE d.penalty_type IN ('level', 'all')
		  AND d.penalty_period > 0
		  AND d.penalty_period != -1
		  AND m.mb_level <= 1
		  AND DATE_ADD(d.penalty_date_from, INTERVAL d.penalty_period DAY) < NOW()
	`).Scan(&expired).Error; err != nil {
		return nil, err
	}

	for _, e := range expired {
		if e.PrevLevel <= 1 {
			e.PrevLevel = 2 // minimum restore level
		}
		if err := db.Table("g5_member").
			Where("mb_id = ? AND mb_level <= 1", e.PenaltyMbID).
			Update("mb_level", e.PrevLevel).Error; err != nil {
			log.Printf("[Cron:discipline-release] failed to restore level for %s: %v", e.PenaltyMbID, err)
			continue
		}
		result.LevelRestoredIDs = append(result.LevelRestoredIDs, e.PenaltyMbID)
		result.LevelRestoredCount++
	}

	// 2. Clear expired intercept dates
	var interceptIDs []string
	if err := db.Raw(`
		SELECT mb_id FROM g5_member
		WHERE mb_intercept_date != '' AND mb_intercept_date != '0000-00-00'
		  AND mb_intercept_date < ?
		  AND mb_intercept_date NOT LIKE '9999%'
	`, now.Format("20060102")).Scan(&interceptIDs).Error; err != nil {
		return nil, err
	}

	if len(interceptIDs) > 0 {
		if err := db.Table("g5_member").
			Where("mb_id IN ?", interceptIDs).
			Update("mb_intercept_date", "").Error; err != nil {
			log.Printf("[Cron:discipline-release] failed to clear intercept dates: %v", err)
		} else {
			result.InterceptReleasedIDs = interceptIDs
			result.InterceptReleasedCount = len(interceptIDs)
		}
	}

	return result, nil
}
