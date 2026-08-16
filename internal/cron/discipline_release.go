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

// shouldReleaseIntercept 는 이용제한을 지금 풀어야 하는지 판정한다.
//
// authoritative 는 g5_da_member_discipline 이 말하는 **통보된 종료 시각**이고,
// interceptDate 는 mb_intercept_date(varchar(8) YYYYMMDD)라는 파생값이다.
// 정본이 있으면 정본을 쓴다 — 파생값은 시각을 담지 못해 하루 단위로 어긋난다.
// 정본이 없으면(수동 SQL 집행 등) 종전대로 파생값으로 판정한다.
func shouldReleaseIntercept(now time.Time, interceptDate string, authoritativeEnd time.Time, hasAuthoritative bool) bool {
	if hasAuthoritative {
		return now.After(authoritativeEnd)
	}
	banEnd, err := parseInterceptDateForCron(interceptDate)
	if err != nil {
		// 형식을 모르면 풀지 않는다 — 판정 불가가 해제가 되면 제재가 무력화된다.
		return false
	}
	return now.After(banEnd)
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

	// ⭐ 통보된 종료 시각을 정본으로 삼는다(2026-08-17, bug: 하루 초과 구금).
	//
	// mb_intercept_date 는 varchar(8) YYYYMMDD 라 시각을 담지 못하는 **파생값**이고,
	// 실측상 처분 종료일보다 하루 뒤가 들어가 있다(콘솔 생성분 전수). 그 값을 다시
	// "그 날 23:59:59 까지"로 해석하면 가산이 두 번 일어나 회원이 **통보받은 시각보다
	// 하루 넘게 더** 갇힌다. 2026-08-17 00:00 시점에 3명이 이 상태였고
	// (노사모준 log4245 1h23m / 하우디 log4241 2h20m / 이글스톤웍스 log4293 5h28m 초과),
	// 그중 한 분이 "이용제한 시간이 지났는데 글을 쓸 수 없다"고 메일로 문의했다.
	//
	// g5_da_member_discipline.penalty_date_from + penalty_period 가 회원에게 실제로
	// 통보된 종료 시각이며(정형 쪽지 「기간: N일 ~ ...」에 그대로 쓰인다), 바로 아래
	// 만료 레코드 정리도 이미 이 기준을 쓴다. 해제 판정만 파생값을 보고 있어 어긋났다.
	//
	// ⛔ 정본이 없는 경우(수동 SQL 집행 등 discipline 행 부재)에는 종전처럼
	//    mb_intercept_date 로 판정한다 — 정본이 없다고 풀어버리면 안 된다.
	type penaltyRow struct {
		MbID   string    `gorm:"column:penalty_mb_id"`
		BanEnd time.Time `gorm:"column:ban_end"`
	}
	var penalties []penaltyRow
	if err := db.Raw(`
		SELECT penalty_mb_id, DATE_ADD(penalty_date_from, INTERVAL penalty_period DAY) AS ban_end
		FROM g5_da_member_discipline
		WHERE penalty_period > 0
	`).Scan(&penalties).Error; err != nil {
		return nil, err
	}
	authoritative := make(map[string]time.Time, len(penalties))
	for _, p := range penalties {
		authoritative[p.MbID] = p.BanEnd
	}

	var interceptIDs []string
	for _, c := range candidates {
		authEnd, hasAuth := authoritative[c.MbID]
		if shouldReleaseIntercept(now, c.InterceptDate, authEnd, hasAuth) {
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
	// ⛔ 이 DELETE 는 반드시 위 해제 판정 **뒤에** 있어야 한다. 먼저 지우면
	//    해제 판정이 정본을 잃고 파생값(mb_intercept_date)으로 되돌아간다.
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
