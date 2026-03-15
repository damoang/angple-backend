package cron

import (
	"log"
	"time"

	"gorm.io/gorm"
)

// DisciplineReleaseResult contains the result of discipline release
type DisciplineReleaseResult struct {
	LevelRestoredCount     int      `json:"level_restored_count"`
	LevelRestoredIDs       []string `json:"level_restored_ids"`
	InterceptReleasedCount int      `json:"intercept_released_count"`
	InterceptReleasedIDs   []string `json:"intercept_released_ids"`
	ExecutedAt             string   `json:"executed_at"`
}

// runDisciplineRelease restores level and clears intercept for expired discipline records
func runDisciplineRelease(db *gorm.DB) (*DisciplineReleaseResult, error) {
	now := time.Now()
	result := &DisciplineReleaseResult{
		ExecutedAt: now.Format("2006-01-02 15:04:05"),
	}

	// mb_level 복구는 더 이상 수행하지 않음 (제재 시 레벨 강등을 하지 않으므로)
	// LevelRestoredCount, LevelRestoredIDs는 항상 0/empty (struct 호환성 유지)

	// Clear expired intercept dates
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
