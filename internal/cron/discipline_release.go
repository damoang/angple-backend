package cron

import (
	"fmt"
	"log"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	cronInterceptDateFormat      = "2006-01-02 15:04:05"
	cronInterceptDateDashFormat  = "2006-01-02"
	cronInterceptDateShortFormat = "20060102"
)

// cronKST 는 해제 판정에 쓰는 로케이션이다.
//
// ⛔ 런타임의 time.Local 에 기대지 않는다. 컨테이너에 TZ=Asia/Seoul 과 zoneinfo 가
// 모두 있고 DSN 에도 loc=Asia/Seoul 이 있는데도, 실측상 해제가 **정확히 9시간**
// 늦었다(2026-09-04). 만료 시각 + 9시간 직후 첫 정각에 풀리는 패턴이 전 건에서
// 일관됐다 — 시각 값이 UTC 로 해석된다는 뜻이다.
//
// mb_intercept_date 와 penalty_date_from 은 모두 KST 벽시계로 기록되므로,
// 환경 설정에 의존하지 않도록 로케이션을 코드에 고정한다.
var cronKST = time.FixedZone("KST", 9*60*60)

// parseInterceptDateForCron parses mb_intercept_date (YYYYMMDD or YYYY-MM-DD HH:MM:SS)
func parseInterceptDateForCron(s string) (time.Time, error) {
	if t, err := time.ParseInLocation(cronInterceptDateFormat, s, cronKST); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation(cronInterceptDateDashFormat, s, cronKST); err == nil {
		return t.Add(24*time.Hour - time.Second), nil
	}
	if t, err := time.ParseInLocation(cronInterceptDateShortFormat, s, cronKST); err == nil {
		return t.Add(24*time.Hour - time.Second), nil
	}
	return time.Time{}, fmt.Errorf("unknown date format: %s", s)
}

// parseBanEndKST 는 DB 가 돌려준 종료 시각 문자열을 KST 로 읽는다.
// ⛔ time.Time 으로 바로 스캔하면 드라이버 설정에 따라 UTC 로 라벨링되어
//
//	같은 9시간 어긋남이 생긴다. 문자열로 받아 여기서만 해석한다.
func parseBanEndKST(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "."); i > 0 { // "2026-09-04 00:36:05.000000"
		s = s[:i]
	}
	return time.ParseInLocation(cronInterceptDateFormat, s, cronKST)
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
	// ⛔ BanEnd 를 time.Time 으로 스캔하지 않는다 — 드라이버가 UTC 로 라벨링해
	//    KST 벽시계 값이 9시간 어긋난다. 문자열로 받아 parseBanEndKST 로 읽는다.
	type penaltyRow struct {
		MbID   string `gorm:"column:penalty_mb_id"`
		BanEnd string `gorm:"column:ban_end"`
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
		end, parseErr := parseBanEndKST(p.BanEnd)
		if parseErr != nil {
			// 형식을 못 읽으면 정본으로 쓰지 않는다 — 파생값 판정으로 떨어진다.
			// ⛔ 여기서 임의로 풀어주면 제재가 무력화된다.
			log.Printf("[Cron:discipline-release] ban_end 파싱 실패 (%s): %v", p.MbID, parseErr)
			continue
		}
		authoritative[p.MbID] = end
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
			recordReleaseOnLog(db, interceptIDs)
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

// recordReleaseOnLog 은 해제된 회원의 가장 최근 이용제한 기록에 실제 해제 시각을 남긴다.
//
// 만료 시 mb_intercept_date 와 g5_da_member_discipline 이 정리되기만 해서
// 사후에 실제 해제 시점을 확인할 근거가 없었다. released_at 을 남기면
// penalty_date_from·penalty_period 와 대조할 수 있다.
//
// 이용자에게 노출되는 member_reason 은 변경하지 않는다. 감사 기록 용도다.
func recordReleaseOnLog(db *gorm.DB, mbIDs []string) {
	for _, mb := range mbIDs {
		res := db.Exec(`
			UPDATE g5_write_disciplinelog
			   SET wr_content = JSON_SET(wr_content,
			         '$.released_at', DATE_FORMAT(CONVERT_TZ(NOW(),'+00:00','+09:00'),'%Y-%m-%d %H:%i:%s'),
			         '$.released_by', 'cron')
			 WHERE JSON_VALID(wr_content)
			   AND JSON_UNQUOTE(JSON_EXTRACT(wr_content,'$.penalty_mb_id')) = ?
			   AND JSON_EXTRACT(wr_content,'$.released_at') IS NULL
			   AND IFNULL(wr_4,'') <> 'void'
			 ORDER BY wr_datetime DESC
			 LIMIT 1`, mb)
		if res.Error != nil {
			log.Printf("[Cron:discipline-release] failed to stamp released_at for %s: %v", mb, res.Error)
		}
	}
}
