package service

import (
	"fmt"
	"log"
	"strings"
	"time"

	"gorm.io/gorm"
)

// 신고 누적 잠금 시 작성자 임시 제한(냉각).
//
// ⛔이것은 제재가 아니라 냉각이다. 징계 기록·사다리에 올리지 않는다.
// 과거 mb_4='lock' 방식은 (1) 강제가 누락돼 아무것도 막지 못했고 (2) 해제 cron(0 8-23)에
// 좌우돼 형기가 1분~9시간(최대 540배)으로 흔들려 작성자가 사실상 형량을 고를 수 있었다.
// 여기서는 **만료 시각(timestamp)** 을 저장해 cron 없이 정확히 만료시킨다.
//
// 구간별 길이(운영자 검토 가능 시간대를 반영):
//   - 주간: 고정 N분(기본 60)
//   - 야간: 다음 해제 시각까지(기본 09:00) — 심야에는 운영자 검토가 어렵다
//   - 주말: 기본은 야간과 동일 규칙(A). kv 에 주말 해제 시각을 넣으면 별도 규칙(B)로 전환된다.
//
// ⚠️상한을 코드에 고정한다. 설정이 잘못돼도 냉각이 무기록 제재로 커지지 않게 한다.
const (
	freezeMaxDay     = 6 * time.Hour  // 주간 상한
	freezeMaxNight   = 12 * time.Hour // 야간 상한
	freezeMaxWeekend = 12 * time.Hour // 주말 상한
)

// FreezeConfig 는 kv_store 에서 읽은 냉각 설정이다.
type FreezeConfig struct {
	DayMinutes   int    // system:report_freeze_minutes (0 = 비활성)
	NightStart   string // system:report_freeze_night_start  "00:00"
	NightUntil   string // system:report_freeze_night_until  "09:00" (빈값 = 야간도 주간 규칙)
	WeekendUntil string // system:report_freeze_weekend_until (빈값 = 주말도 야간/주간 규칙 = A)
}

func kvText(db *gorm.DB, key string) string {
	var r struct {
		ValueType string `gorm:"column:value_type"`
		ValueText string `gorm:"column:value_text"`
		ValueInt  int    `gorm:"column:value_int"`
	}
	if err := db.Raw(
		"SELECT value_type, value_text, value_int FROM g5_kv_store WHERE `key` = ? LIMIT 1", key,
	).Scan(&r).Error; err != nil {
		return ""
	}
	if r.ValueType == "INT" {
		return fmt.Sprintf("%d", r.ValueInt)
	}
	return strings.TrimSpace(r.ValueText)
}

// LoadFreezeConfig 는 kv_store 에서 냉각 설정을 읽는다. 미설정이면 비활성(0분).
func LoadFreezeConfig(db *gorm.DB) FreezeConfig {
	cfg := FreezeConfig{
		NightStart:   kvText(db, "system:report_freeze_night_start"),
		NightUntil:   kvText(db, "system:report_freeze_night_until"),
		WeekendUntil: kvText(db, "system:report_freeze_weekend_until"),
	}
	if v := kvText(db, "system:report_freeze_minutes"); v != "" {
		var m int
		if _, err := fmt.Sscanf(v, "%d", &m); err == nil && m > 0 {
			cfg.DayMinutes = m
		}
	}
	return cfg
}

// parseHM 은 "09:00" 을 시/분으로 판다. 실패하면 ok=false.
func parseHM(s string) (h, m int, ok bool) {
	if _, err := fmt.Sscanf(strings.TrimSpace(s), "%d:%d", &h, &m); err != nil {
		return 0, 0, false
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, false
	}
	return h, m, true
}

// nextAt 은 now 이후 가장 가까운 h:m 시각을 돌려준다(오늘 지났으면 내일).
func nextAt(now time.Time, h, m int) time.Time {
	t := time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, now.Location())
	if !t.After(now) {
		t = t.AddDate(0, 0, 1)
	}
	return t
}

// isNight 은 now 가 야간 구간(nightStart ~ nightUntil)인지 본다.
func isNight(now time.Time, cfg FreezeConfig) bool {
	sh, sm, okS := parseHM(cfg.NightStart)
	uh, um, okU := parseHM(cfg.NightUntil)
	if !okS || !okU {
		return false
	}
	cur := now.Hour()*60 + now.Minute()
	start := sh*60 + sm
	until := uh*60 + um
	if start <= until {
		return cur >= start && cur < until
	}
	// 자정을 넘는 구간(예: 23:00~09:00)
	return cur >= start || cur < until
}

// ComputeFreezeUntil 은 지금 잠긴 작성자의 냉각 만료 시각을 계산한다.
// 비활성이거나 계산 불가면 zero time 을 돌려준다(=냉각 없음).
func ComputeFreezeUntil(now time.Time, cfg FreezeConfig) time.Time {
	if cfg.DayMinutes <= 0 {
		return time.Time{} // 비활성(킬스위치)
	}

	weekend := now.Weekday() == time.Saturday || now.Weekday() == time.Sunday

	// B: 주말 규칙이 설정돼 있으면 우선 적용(빈값이면 A = 주말도 아래 규칙 그대로)
	if weekend && cfg.WeekendUntil != "" {
		if h, m, ok := parseHM(cfg.WeekendUntil); ok {
			until := nextAt(now, h, m)
			if until.Sub(now) > freezeMaxWeekend {
				until = now.Add(freezeMaxWeekend)
			}
			return until
		}
	}

	// 야간: 해제 시각까지(운영자 검토 가능 시점)
	if isNight(now, cfg) {
		if h, m, ok := parseHM(cfg.NightUntil); ok {
			until := nextAt(now, h, m)
			if until.Sub(now) > freezeMaxNight {
				until = now.Add(freezeMaxNight)
			}
			return until
		}
	}

	// 주간: 고정 N분
	d := time.Duration(cfg.DayMinutes) * time.Minute
	if d > freezeMaxDay {
		d = freezeMaxDay
	}
	return now.Add(d)
}

// ApplyReportFreeze 는 작성자에게 냉각을 건다. 이미 더 긴 냉각이 있으면 연장하지 않는다.
// 실패는 잠금 자체를 실패시키지 않는다(로그만).
func ApplyReportFreeze(db *gorm.DB, mbID string, reason string) {
	if mbID == "" {
		return
	}
	cfg := LoadFreezeConfig(db)
	until := ComputeFreezeUntil(time.Now(), cfg)
	if until.IsZero() {
		return
	}
	if err := db.Exec(`
		INSERT INTO g5_member_freeze (mb_id, frozen_until, reason)
		VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE
			frozen_until = GREATEST(frozen_until, VALUES(frozen_until)),
			reason = VALUES(reason)
	`, mbID, until, reason).Error; err != nil {
		log.Printf("[freeze] 적용 실패 (%s): %v", mbID, err)
		return
	}
	log.Printf("[freeze] %s frozen until %s (reason=%s)", mbID, until.Format(time.RFC3339), reason)
}

// FrozenUntil 은 회원이 냉각 중이면 만료 시각을, 아니면 zero time 을 돌려준다.
// 만료된 행은 조회에서 자연히 제외되므로 해제 cron 이 필요 없다.
func FrozenUntil(db *gorm.DB, mbID string) time.Time {
	if mbID == "" {
		return time.Time{}
	}
	var until time.Time
	if err := db.Raw(
		`SELECT frozen_until FROM g5_member_freeze WHERE mb_id = ? AND frozen_until > NOW() LIMIT 1`,
		mbID,
	).Scan(&until).Error; err != nil {
		return time.Time{}
	}
	return until
}
