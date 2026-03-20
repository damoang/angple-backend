package cron

import (
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"
)

const (
	cronInterceptDateFormat      = "2006-01-02 15:04:05"
	cronInterceptDateDashFormat  = "2006-01-02"
	cronInterceptDateShortFormat = "20060102"
)

// parseInterceptDateForCron parses mb_intercept_date (YYYYMMDD or YYYY-MM-DD HH:MM:SS)
func parseInterceptDateForCron(s string) (time.Time, error) {
	if t, err := time.ParseInLocation(cronInterceptDateFormat, s, time.Local); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation(cronInterceptDateDashFormat, s, time.Local); err == nil {
		return t.Add(24*time.Hour - time.Second), nil
	}
	if t, err := time.ParseInLocation(cronInterceptDateShortFormat, s, time.Local); err == nil {
		return t.Add(24*time.Hour - time.Second), nil
	}
	return time.Time{}, fmt.Errorf("unknown date format: %s", s)
}

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
	// 형식이 혼재(YYYYMMDD, YYYY-MM-DD HH:MM:SS)할 수 있으므로 모든 후보를 로드 → Go에서 파싱
	type interceptRow struct {
		MbID          string `gorm:"column:mb_id"`
		InterceptDate string `gorm:"column:mb_intercept_date"`
	}
	var candidates []interceptRow
	if err := db.Raw(`
		SELECT mb_id, mb_intercept_date FROM g5_member
		WHERE mb_intercept_date != '' AND mb_intercept_date != '0000-00-00'
		  AND mb_intercept_date NOT LIKE '9999%'
	`).Scan(&candidates).Error; err != nil {
		return nil, err
	}

	var interceptIDs []string
	for _, c := range candidates {
		banEnd, err := parseInterceptDateForCron(c.InterceptDate)
		if err != nil {
			continue
		}
		if now.After(banEnd) {
			interceptIDs = append(interceptIDs, c.MbID)
		}
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

	// Clean up expired discipline records from g5_da_member_discipline
	// (prevents fallback in ban_check from re-applying expired restrictions)
	expiredResult := db.Exec(`
		DELETE FROM g5_da_member_discipline
		WHERE penalty_period > 0
		  AND DATE_ADD(penalty_date_from, INTERVAL penalty_period DAY) < NOW()
	`)
	if expiredResult.Error != nil {
		log.Printf("[Cron:discipline-release] failed to clean expired discipline records: %v", expiredResult.Error)
	} else if expiredResult.RowsAffected > 0 {
		log.Printf("[Cron:discipline-release] cleaned %d expired discipline records", expiredResult.RowsAffected)
	}

	return result, nil
}
