package cron

import (
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

// 담당 일치 자동 집행(2·3호).
//
// 유효 담당자 N명(기본 2) 이상이 모두 처분 의견이고 사유가 2호(22)·3호(23)뿐인 신고를
// 시스템이 승인(admin_approved=1)해 둔다. 승인된 행은 같은 실행의 process-approved-reports
// 본 처리(로그 작성·제한 적용·쪽지·processed 표시)가 그대로 집행한다.
//
// singo_settings 키
//   - auto_execute_mode: off | dry_run | on (없음·알 수 없는 값 = off)
//   - auto_execute_min_opinions: 최소 일치 담당 수 (기본 2, 2 미만으로는 내려가지 않는다)
//
// 자동 미처리(auto_dismiss_min_opinions)와 기준을 따로 둔다. 기각과 집행은 무게가 달라
// 한쪽 기준을 바꿀 때 다른 쪽이 따라 움직이면 안 된다.

const (
	autoExecuteModeOff    = "off"
	autoExecuteModeDryRun = "dry_run"
	autoExecuteModeOn     = "on"

	autoExecuteDefaultMinOpinions = 2

	reasonCodeNoManner      = 22 // 제9조 제1항 제2호 (예의없음)
	reasonCodeInappropriate = 23 // 제9조 제1항 제3호 (부적절한 표현)

	autoExecuteAdminID = "system-auto"
	autoExecuteDetail  = "담당 2인 일치 자동 집행(2·3호)"
)

// autoExecuteTableRe 는 동적 테이블명(g5_write_{bo_table})에 넣을 수 있는 게시판 ID 패턴이다.
var autoExecuteTableRe = regexp.MustCompile(`^[a-zA-Z0-9_]{1,30}$`)

// autoExecuteNonHumanReviewers 는 담당 수에 세지 않는 자동화 계정이다.
// 이 계정의 의견도 사유·미처리 판정에는 그대로 반영된다(보수 방향).
var autoExecuteNonHumanReviewers = map[string]bool{"ai": true}

// AutoExecuteMember 는 피신고 회원 1명 단위의 자동 집행(또는 시험) 결과다.
type AutoExecuteMember struct {
	TargetMbID string   `json:"target_mb_id"`
	Items      []string `json:"items"` // "table:sg_id:sg_parent"
	Reasons    []int    `json:"reasons"`
	Days       int      `json:"days"`  // 0=주의
	Round      int      `json:"round"` // 기존 이력 수 + 1
	Applied    bool     `json:"applied"`
	SkipReason string   `json:"skip_reason,omitempty"` // 회원 단위 제외 사유(예: member_active_restriction)
	Error      string   `json:"error,omitempty"`
}

// AutoExecuteResult 는 자동 집행 단계의 실행 결과다.
type AutoExecuteResult struct {
	Mode           string              `json:"mode"`
	MinOpinions    int                 `json:"min_opinions"`
	CandidateUnits int                 `json:"candidate_units"`
	SelectedUnits  int                 `json:"selected_units"`
	Skipped        map[string]int      `json:"skipped,omitempty"`
	Members        []AutoExecuteMember `json:"members,omitempty"`
	Errors         int                 `json:"errors"`
}

// parseAutoExecuteMode 는 설정값을 모드로 정규화한다. 알 수 없는 값은 off.
func parseAutoExecuteMode(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case autoExecuteModeOn:
		return autoExecuteModeOn
	case autoExecuteModeDryRun:
		return autoExecuteModeDryRun
	default:
		return autoExecuteModeOff
	}
}

// parseAutoExecuteMinOpinions 는 최소 일치 담당 수를 정규화한다(기본 2, 하한 2).
func parseAutoExecuteMinOpinions(v string) int {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < autoExecuteDefaultMinOpinions {
		return autoExecuteDefaultMinOpinions
	}
	return n
}

// parseHo23Reasons 는 의견의 discipline_reasons(콤마 구분 또는 JSON 배열)를 읽어
// 비어 있지 않고 모두 22·23 인지 판정한다. 그 밖의 값(다른 사유, 문자 키, 구 코드)이
// 하나라도 있으면 false — 판정할 수 없는 값은 제외 쪽으로 처리한다.
func parseHo23Reasons(raw string) ([]int, bool) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	if strings.TrimSpace(s) == "" {
		return nil, false
	}
	var out []int
	for _, tok := range strings.Split(s, ",") {
		tok = strings.Trim(strings.TrimSpace(tok), `"`)
		if tok == "" {
			continue
		}
		n, err := strconv.Atoi(tok)
		if err != nil || (n != reasonCodeNoManner && n != reasonCodeInappropriate) {
			return nil, false
		}
		out = unionInts(out, []int{n})
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

// autoExecOpinion 은 정규화된(담당 회원 ID로 바꾼) 유효 의견 1건이다.
type autoExecOpinion struct {
	Reviewer string // 담당 회원 ID (mb_id)
	Type     string // action | dismiss
	Reasons  string
	Days     int
}

// autoExecUnitEval 은 신고 단위(sg) 하나의 판정 결과다.
type autoExecUnitEval struct {
	OK        bool
	Skip      string // 제외 사유 키
	Reasons   []int
	AllZero   bool     // 처분 의견이 모두 0일(주의)
	Reviewers []string // 의견을 낸 담당 전원 (이해충돌 검사 대상)
}

// evaluateAutoExecUnit 은 한 신고 단위의 유효 의견으로 자동 집행 대상 여부를 판정한다(순수 함수).
func evaluateAutoExecUnit(ops []autoExecOpinion, minOpinions int) autoExecUnitEval {
	ev := autoExecUnitEval{AllZero: true}
	actionReviewers := map[string]bool{}
	allReviewers := map[string]bool{}
	for _, o := range ops {
		if o.Reviewer == "" {
			return autoExecUnitEval{Skip: "unknown_reviewer"}
		}
		allReviewers[o.Reviewer] = true
		switch o.Type {
		case "dismiss":
			return autoExecUnitEval{Skip: "has_dismiss"}
		case "action":
			reasons, ok := parseHo23Reasons(o.Reasons)
			if !ok {
				return autoExecUnitEval{Skip: "other_reason"}
			}
			// 자동화 계정 의견은 사유 판정(제외 조건)에만 쓰고, 담당 수·사유 합집합·주의 판정에는 넣지 않는다.
			if autoExecuteNonHumanReviewers[o.Reviewer] {
				continue
			}
			actionReviewers[o.Reviewer] = true
			ev.Reasons = unionInts(ev.Reasons, reasons)
			if o.Days != 0 {
				ev.AllZero = false
			}
		default:
			return autoExecUnitEval{Skip: "unknown_opinion"}
		}
	}
	if len(actionReviewers) < minOpinions {
		return autoExecUnitEval{Skip: "not_enough_opinions"}
	}
	for r := range allReviewers {
		ev.Reviewers = append(ev.Reviewers, r)
	}
	sort.Strings(ev.Reviewers)
	sort.Ints(ev.Reasons)
	ev.OK = true
	return ev
}

// hasReviewerConflict 는 담당 중 누구라도 그 스레드(원글·댓글)의 작성자인지 본다(순수 함수).
func hasReviewerConflict(reviewers, threadAuthors []string) bool {
	authors := make(map[string]bool, len(threadAuthors))
	for _, a := range threadAuthors {
		if a = strings.TrimSpace(a); a != "" {
			authors[a] = true
		}
	}
	for _, r := range reviewers {
		if authors[r] {
			return true
		}
	}
	return false
}

// autoExecuteLadderDays 는 회차별 원칙 일수다. 7회차 이상은 영구(permanentDays).
func autoExecuteLadderDays(round int) int {
	switch {
	case round <= 1:
		return 1
	case round == 2:
		return 5
	case round == 3:
		return 10
	case round == 4:
		return 30
	case round == 5:
		return 180
	case round == 6:
		return 365
	default:
		return permanentDays
	}
}

// computeAutoExecuteDays 는 자동 집행 일수를 계산한다(순수 함수).
//   - 처분 의견이 모두 0일 → 0(주의)
//   - 그 밖에는 회차 원칙 일수에 상한: 2호만 1일, 3호 포함 5일
//
// 결과는 0·1·5 중 하나이며 이 경로에서 영구는 나오지 않는다.
func computeAutoExecuteDays(priorCount int, reasons []int, allZero bool) int {
	if allZero {
		return 0
	}
	days := autoExecuteLadderDays(priorCount + 1)
	limit := 1
	for _, r := range reasons {
		if r == reasonCodeInappropriate {
			limit = 5
		}
	}
	if days > limit {
		days = limit
	}
	return days
}

// buildAutoExecuteMemberReason 은 회원 공개 사유를 만든다. 운영정책 문구를 그대로 인용한다.
func buildAutoExecuteMemberReason(reasons []int, days int) string {
	has2, has3 := false, false
	for _, r := range reasons {
		switch r {
		case reasonCodeNoManner:
			has2 = true
		case reasonCodeInappropriate:
			has3 = true
		}
	}

	const quote2 = "제2호(예의없음) 「반말 등 예의를 갖추지 않은 행위」"
	const quote3 = "제3호(부적절한 표현) 「욕설, 비속어, 혐오표현 등 부적절한 표현을 사용하는 행위」"

	var clause string
	switch {
	case has2 && has3:
		clause = "운영정책 제9조 제1항 " + quote2 + " 및 " + quote3
	case has3:
		clause = "운영정책 제9조 제1항 " + quote3
	default:
		clause = "운영정책 제9조 제1항 " + quote2
	}

	result := "이에 따라 주의 조치되었으며, 이용에는 제한이 없습니다."
	if days > 0 {
		result = fmt.Sprintf("이에 따라 이용제한 %d일이 적용되었습니다.", days)
	}

	var b strings.Builder
	b.WriteString("회원님께서 작성하신 게시물에 대해 안내드립니다.\n\n")
	b.WriteString("신고된 게시물이 " + clause + "에 해당하는 것으로 확인되었습니다.\n\n")
	if has2 {
		b.WriteString("저희 운영정책은 모든 게시물과 댓글에서 경어체를 사용하도록 정하고 있습니다.\n\n")
	}
	b.WriteString(result + "\n\n")
	b.WriteString("이 처분에 이의가 있으시면 소명게시판을 이용하실 수 있습니다.")
	return b.String()
}

// legacySubjectLikePattern 은 구 형식 제목("id 경고", "id 5일" 등)을 찾는 LIKE 패턴을 만든다.
// 회원 ID 안의 LIKE 와일드카드(_ %)와 이스케이프 문자(!)는 '!' 로 이스케이프한다(ESCAPE '!').
func legacySubjectLikePattern(mbID string) string {
	r := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_")
	return r.Replace(mbID) + " %"
}

// isActiveRestriction 은 mb_intercept_date(이용제한 종료일, YYYYMMDD)가 오늘(KST, YYYYMMDD) 이후인지 본다.
// 종료일이 오늘인 경우도 아직 제한 중으로 본다. 빈 값은 제한 없음. 형식을 알 수 없는 값은 판정할 수 없으므로 제한 중으로 본다(fail closed).
func isActiveRestriction(interceptDate, todayKST string) bool {
	d := strings.TrimSpace(interceptDate)
	if d == "" {
		return false
	}
	if len(d) != 8 {
		return true
	}
	for _, c := range d {
		if c < '0' || c > '9' {
			return true
		}
	}
	return d >= todayKST
}

// reasonsJSON 은 사유 코드를 admin_discipline_reasons 형식("[22,23]")으로 만든다.
func reasonsJSON(reasons []int) string {
	parts := make([]string, len(reasons))
	for i, r := range reasons {
		parts[i] = strconv.Itoa(r)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

type autoExecUnit struct {
	Table    string `gorm:"column:sg_table"`
	SGID     int    `gorm:"column:sg_id"`
	Parent   int    `gorm:"column:sg_parent"`
	TargetLo string `gorm:"column:target_lo"`
	TargetHi string `gorm:"column:target_hi"`
}

func (u autoExecUnit) key() string { return fmt.Sprintf("%s:%d:%d", u.Table, u.SGID, u.Parent) }

type autoExecSelected struct {
	unit    autoExecUnit
	reasons []int
	allZero bool
}

// runAutoApproveReports 는 자동 집행 대상을 골라 mode=on 이면 승인 상태로 기록한다.
// 실패해도 본 처리(runProcessApprovedReports)를 막지 않도록 호출부에서 로그만 남긴다.
func runAutoApproveReports(db *gorm.DB) (*AutoExecuteResult, error) {
	res := &AutoExecuteResult{Mode: autoExecuteModeOff, MinOpinions: autoExecuteDefaultMinOpinions, Skipped: map[string]int{}}

	var modeStr string
	db.Raw("SELECT `value` FROM singo_settings WHERE `key` = ?", "auto_execute_mode").Scan(&modeStr)
	res.Mode = parseAutoExecuteMode(modeStr)
	if res.Mode == autoExecuteModeOff {
		return res, nil
	}

	var minStr string
	db.Raw("SELECT `value` FROM singo_settings WHERE `key` = ?", "auto_execute_min_opinions").Scan(&minStr)
	res.MinOpinions = parseAutoExecuteMinOpinions(minStr)

	// 1. 후보 신고 단위 — 자동 미처리와 같은 자격(모니터링 완료·미승인·미처리·미보류).
	var units []autoExecUnit
	if err := db.Raw(`
		SELECT s.sg_table, s.sg_id, s.sg_parent,
			MIN(s.target_mb_id) AS target_lo, MAX(s.target_mb_id) AS target_hi
		FROM g5_na_singo s
		JOIN (
			SELECT DISTINCT sg_table, sg_id, sg_parent
			FROM g5_na_singo
			WHERE processed = 0 AND admin_approved = 0 AND hold = 0 AND monitoring_checked = 1
		) u ON s.sg_table = u.sg_table AND s.sg_id = u.sg_id AND s.sg_parent = u.sg_parent
		GROUP BY s.sg_table, s.sg_id, s.sg_parent
		HAVING MAX(s.monitoring_checked) = 1
		   AND MAX(s.admin_approved) = 0
		   AND MAX(s.processed) = 0
		   AND MAX(s.hold) = 0
	`).Scan(&units).Error; err != nil {
		return res, fmt.Errorf("자동 집행 후보 조회 실패: %w", err)
	}
	res.CandidateUnits = len(units)
	if len(units) == 0 {
		return res, nil
	}

	reviewerNames := map[string]string{} // reviewer_id → mb_id 캐시
	resolveReviewer := func(id string) string {
		id = strings.TrimSpace(id)
		if id == "" {
			return ""
		}
		if v, ok := reviewerNames[id]; ok {
			return v
		}
		name := id
		// reviewer_id 는 v2_users.id(숫자) 또는 username(=mb_id) 두 형식이 섞여 있다.
		if n, err := strconv.ParseInt(id, 10, 64); err == nil && n > 0 {
			var username string
			db.Raw("SELECT username FROM v2_users WHERE id = ?", n).Scan(&username)
			name = strings.TrimSpace(username) // 못 찾으면 "" → 판정 불가로 제외
		}
		reviewerNames[id] = name
		return name
	}

	// 2. 단위별 판정
	byMember := map[string][]autoExecSelected{}
	memberOrder := make([]string, 0, len(units))
	for _, u := range units {
		if !autoExecuteTableRe.MatchString(u.Table) {
			res.Skipped["invalid_table"]++
			continue
		}
		target := strings.TrimSpace(u.TargetHi)
		if target == "" || strings.TrimSpace(u.TargetLo) != target {
			res.Skipped["unknown_target"]++
			continue
		}

		type opRow struct {
			ReviewerID string `gorm:"column:reviewer_id"`
			Type       string `gorm:"column:opinion_type"`
			Reasons    string `gorm:"column:discipline_reasons"`
			Days       int    `gorm:"column:discipline_days"`
		}
		var rows []opRow
		if err := db.Raw(`
			SELECT reviewer_id, opinion_type, IFNULL(discipline_reasons, '') AS discipline_reasons,
				IFNULL(discipline_days, 0) AS discipline_days
			FROM g5_na_singo_opinions
			WHERE sg_table = ? AND sg_id = ? AND sg_parent = ? AND is_valid_reviewer = 1
		`, u.Table, u.SGID, u.Parent).Scan(&rows).Error; err != nil {
			res.Errors++
			continue
		}
		if len(rows) == 0 {
			res.Skipped["no_opinions"]++
			continue
		}
		ops := make([]autoExecOpinion, 0, len(rows))
		for _, r := range rows {
			ops = append(ops, autoExecOpinion{
				Reviewer: resolveReviewer(r.ReviewerID),
				Type:     r.Type,
				Reasons:  r.Reasons,
				Days:     r.Days,
			})
		}
		ev := evaluateAutoExecUnit(ops, res.MinOpinions)
		if !ev.OK {
			res.Skipped[ev.Skip]++
			continue
		}

		// 이해충돌: 담당이 원글 작성자이거나 같은 스레드에 댓글을 쓴 경우 제외.
		thread := u.Parent
		if thread <= 0 {
			thread = u.SGID
		}
		var authors []string
		if err := db.Raw(fmt.Sprintf(
			"SELECT DISTINCT mb_id FROM g5_write_%s WHERE wr_id = ? OR wr_parent = ?", u.Table),
			thread, thread).Scan(&authors).Error; err != nil {
			res.Skipped["thread_lookup_failed"]++
			continue
		}
		if hasReviewerConflict(ev.Reviewers, authors) {
			res.Skipped["reviewer_conflict"]++
			continue
		}

		if _, ok := byMember[target]; !ok {
			memberOrder = append(memberOrder, target)
		}
		byMember[target] = append(byMember[target], autoExecSelected{unit: u, reasons: ev.Reasons, allZero: ev.AllZero})
		res.SelectedUnits++
	}

	// 3. 회원 단위로 묶어 1건의 처분으로 계산·기록
	kst := time.FixedZone("KST", 9*60*60)
	for _, target := range memberOrder {
		items := byMember[target]

		keys := make([]string, 0, len(items))
		for _, it := range items {
			keys = append(keys, it.unit.key())
		}
		skipMember := func(reason string) {
			res.Skipped[reason] += len(items)
			res.Members = append(res.Members, AutoExecuteMember{TargetMbID: target, Items: keys, SkipReason: reason})
		}

		// 수동 승인 대기분과 섞이면 한 처분으로 합쳐지므로, 대기분이 있는 회원은 이번 회차에서 제외한다.
		// 조회에 실패하면 판정할 수 없으므로 제외한다(fail closed).
		var pending int64
		if err := db.Raw(`SELECT COUNT(*) FROM g5_na_singo WHERE target_mb_id = ? AND admin_approved = 1 AND processed = 0`, target).
			Scan(&pending).Error; err != nil {
			res.Errors++
			skipMember("pending_lookup_failed")
			continue
		}
		if pending > 0 {
			skipMember("member_has_pending_approval")
			continue
		}

		// 이미 이용제한 중인 회원은 자동으로 겹쳐 집행하지 않는다(운영자 판단 몫). 조회 실패도 제외.
		var intercept string
		if err := db.Raw(`SELECT IFNULL(mb_intercept_date, '') FROM g5_member WHERE mb_id = ?`, target).
			Scan(&intercept).Error; err != nil {
			res.Errors++
			skipMember("member_lookup_failed")
			continue
		}
		if isActiveRestriction(intercept, time.Now().In(kst).Format("20060102")) {
			skipMember("member_active_restriction")
			continue
		}

		var reasons []int
		allZero := true
		for _, it := range items {
			reasons = unionInts(reasons, it.reasons)
			if !it.allZero {
				allZero = false
			}
		}
		sort.Ints(reasons)

		// 기존 이력 수: 이용제한 기록 게시판 본글 중 삭제·회수·무효가 아닌 것. 주의(0일) 기록도 회차에 포함된다.
		// 대상 매칭은 세 형식을 모두 본다 — JSON penalty_mb_id(생성 컬럼), 제목 "id(닉네임)",
		// 구 제목 "id 경고"/"id 5일" 등. LIKE 패턴의 와일드카드(_ %)는 이스케이프한다.
		var prior int64
		if err := db.Raw(`
			SELECT COUNT(*) FROM g5_write_disciplinelog
			WHERE wr_is_comment = 0
			  AND wr_deleted_at IS NULL
			  AND IFNULL(wr_4, '') NOT IN ('revoked', 'void')
			  AND (penalty_mb_id = ?
			       OR SUBSTRING_INDEX(wr_subject, '(', 1) = ?
			       OR wr_subject LIKE ? ESCAPE '!')
		`, target, target, legacySubjectLikePattern(target)).Scan(&prior).Error; err != nil {
			res.Errors++
			skipMember("prior_lookup_failed")
			continue
		}

		days := computeAutoExecuteDays(int(prior), reasons, allZero)
		m := AutoExecuteMember{
			TargetMbID: target,
			Items:      keys,
			Reasons:    reasons,
			Days:       days,
			Round:      int(prior) + 1,
		}

		if res.Mode == autoExecuteModeOn {
			if err := applyAutoExecute(db, items, reasons, days, kst); err != nil {
				m.Error = err.Error()
				res.Errors++
			} else {
				m.Applied = true
			}
		}
		res.Members = append(res.Members, m)
	}

	logAutoExecuteSummary(res)
	return res, nil
}

// applyAutoExecute 는 회원 1명분 신고 단위를 승인 상태로 기록한다.
// 예약 승인과 같은 컬럼을 채우며, 한 단위라도 상태가 바뀌어 있으면 회원 전체를 되돌린다.
func applyAutoExecute(db *gorm.DB, items []autoExecSelected, reasons []int, days int, kst *time.Location) error {
	now := time.Now().In(kst)
	adminUsers, err := json.Marshal([]map[string]string{{"mb_id": autoExecuteAdminID, "datetime": now.Format("2006-01-02 15:04:05")}})
	if err != nil {
		return err
	}
	disciplineType := ""
	if days > 0 {
		disciplineType = "level" // 운영 콘솔 승인 기본값(등급 제한)과 같다
	}
	memberReason := buildAutoExecuteMemberReason(reasons, days)
	reasonStr := reasonsJSON(reasons)

	return db.Transaction(func(tx *gorm.DB) error {
		for _, it := range items {
			u := it.unit
			r := tx.Exec(`
				UPDATE g5_na_singo
				SET admin_approved = 1, processed = 0, hold = 0,
				    admin_datetime = CONVERT_TZ(NOW(), '+00:00', '+09:00'),
				    admin_users = ?,
				    admin_discipline_reasons = ?,
				    admin_discipline_days = ?,
				    admin_discipline_type = ?,
				    admin_discipline_detail = ?,
				    admin_member_reason = ?,
				    version = version + 1
				WHERE sg_table = ? AND sg_id = ? AND sg_parent = ?
				  AND processed = 0 AND admin_approved = 0 AND hold = 0
			`, string(adminUsers), reasonStr, days, disciplineType, autoExecuteDetail, memberReason,
				u.Table, u.SGID, u.Parent)
			if r.Error != nil {
				return r.Error
			}
			if r.RowsAffected == 0 {
				return fmt.Errorf("상태 변경됨: %s", u.key())
			}
			if err := tx.Exec(`
				INSERT INTO g5_singo_history
					(sg_table, sg_id, sg_parent, prev_status, new_status, admin_id, admin_note, created_at)
				VALUES (?, ?, ?, 'monitoring', 'scheduled', ?, ?, CONVERT_TZ(NOW(), '+00:00', '+09:00'))
			`, u.Table, u.SGID, u.Parent, autoExecuteAdminID, autoExecuteDetail).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// logAutoExecuteSummary 는 결과 요약을 로그로 남긴다(시험 모드의 보고 수단).
func logAutoExecuteSummary(res *AutoExecuteResult) {
	log.Printf("[Cron:auto-execute-reports] mode=%s min=%d candidates=%d selected=%d members=%d errors=%d skipped=%v",
		res.Mode, res.MinOpinions, res.CandidateUnits, res.SelectedUnits, len(res.Members), res.Errors, res.Skipped)
	for _, m := range res.Members {
		if m.SkipReason != "" {
			log.Printf("[Cron:auto-execute-reports] mode=%s target=%s items=%v skipped=%s",
				res.Mode, m.TargetMbID, m.Items, m.SkipReason)
			continue
		}
		log.Printf("[Cron:auto-execute-reports] mode=%s target=%s items=%v reasons=%v round=%d days=%d applied=%v err=%s",
			res.Mode, m.TargetMbID, m.Items, m.Reasons, m.Round, m.Days, m.Applied, m.Error)
	}
}
