package cron

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

// AutoDismissResult contains the result of the auto-dismiss cron run.
type AutoDismissResult struct {
	Enabled        bool     `json:"enabled"`
	MinOpinions    int      `json:"min_opinions"`
	CandidateCount int      `json:"candidate_count"`
	DismissedRows  int      `json:"dismissed_rows"`
	DismissedKeys  []string `json:"dismissed_keys"`
	Errors         int      `json:"errors"`
	ExecutedAt     string   `json:"executed_at"`
}

// runAutoDismissReports 자동 미처리(기각) 크론.
//
// 집계·처리 단위는 신고된 개별 댓글/글(sg_id)이다. 의견 저장과 수동 기각(GetAllByTableAndSgID,
// WHERE sg_table=? AND sg_id=?)이 모두 sg_id 단위이므로 자동기각도 동일 단위로 맞춘다.
// (과거 parent 단위 집계는, 한 글 아래 다른 댓글의 이용제한 의견이 무관한 댓글의 기각을 막는 버그가 있었다.)
//
//   - 의견 집계는 반드시 is_valid_reviewer = 1 (현재 유효한 담당자) 인 의견만, COUNT(DISTINCT reviewer_id) 로 센다.
//     (탈퇴/해제된 옛 담당자의 표는 세지 않는다.)
//   - 같은 sg_id에 대해 미조치(dismiss) 유효 담당자 N명(기본 2) 이상 AND 조치(action) 유효 담당자 0명.
//   - sg_id 단위 집계: monitoring_checked=1 AND admin_approved=0 AND processed=0 AND hold=0
//     (이미 admin이 승인/처리했거나 보류한 건은 제외).
//
// singo_settings.auto_dismiss_enabled = 'true' 일 때만 동작한다(기본 비활성).
func runAutoDismissReports(db *gorm.DB) (*AutoDismissResult, error) {
	now := time.Now()
	result := &AutoDismissResult{MinOpinions: 2, ExecutedAt: now.Format("2006-01-02 15:04:05")}

	// 1. 활성화 여부 확인 — 설정이 'true'가 아니면 아무 것도 하지 않음
	var enabled string
	db.Raw("SELECT `value` FROM singo_settings WHERE `key` = ?", "auto_dismiss_enabled").Scan(&enabled)
	if strings.TrimSpace(enabled) != "true" {
		result.Enabled = false
		return result, nil
	}
	result.Enabled = true

	// 2. 최소 미처리 인원 (기본 2, 설정으로 조정 가능)
	var minStr string
	db.Raw("SELECT `value` FROM singo_settings WHERE `key` = ?", "auto_dismiss_min_opinions").Scan(&minStr)
	if n, err := strconv.Atoi(strings.TrimSpace(minStr)); err == nil && n > 0 {
		result.MinOpinions = n
	}

	// 3. 후보 조회 — sg_id(개별 댓글) 단위.
	//    유효 담당자(is_valid_reviewer=1)의 distinct 미조치 N명 이상 + distinct 조치 0명,
	//    그리고 sg_id 단위로 monitoring_checked=1, admin_approved=0, processed=0, hold=0.
	type candidate struct {
		Table  string `gorm:"column:sg_table"`
		SGID   int    `gorm:"column:sg_id"`
		Parent int    `gorm:"column:sg_parent"`
	}
	var candidates []candidate
	if err := db.Raw(`
		SELECT s.sg_table, s.sg_id, s.sg_parent
		FROM g5_na_singo s
		LEFT JOIN (
			SELECT o.sg_table, o.sg_id, o.sg_parent,
				COUNT(DISTINCT CASE WHEN o.opinion_type = 'action' THEN o.reviewer_id END) AS action_count,
				COUNT(DISTINCT CASE WHEN o.opinion_type = 'dismiss' THEN o.reviewer_id END) AS dismiss_count
			FROM g5_na_singo_opinions o
			WHERE o.is_valid_reviewer = 1
			GROUP BY o.sg_table, o.sg_id, o.sg_parent
		) op ON s.sg_table = op.sg_table AND s.sg_id = op.sg_id AND s.sg_parent = op.sg_parent
		GROUP BY s.sg_table, s.sg_id, s.sg_parent
		HAVING MAX(s.monitoring_checked) = 1
		   AND MAX(s.admin_approved) = 0
		   AND MAX(s.processed) = 0
		   AND MAX(s.hold) = 0
		   AND IFNULL(MAX(op.dismiss_count), 0) >= ?
		   AND IFNULL(MAX(op.action_count), 0) = 0
	`, result.MinOpinions).Scan(&candidates).Error; err != nil {
		return result, err
	}
	result.CandidateCount = len(candidates)

	// 4. 후보 댓글(sg_id)별 미처리 신고를 기각 처리 (처리자 system).
	//    범위는 수동 기각(GetAllByTableAndSgID)과 동일하게 sg_table + sg_id.
	note := fmt.Sprintf("자동 미처리 (유효 담당자 만장일치 %d명)", result.MinOpinions)
	for _, cand := range candidates {
		// 기각 처리 — UpdateStatus(dismissed, "system")와 동일한 컬럼 세트
		res := db.Exec(`
			UPDATE g5_na_singo
			SET processed = 1, admin_approved = 0, hold = 0,
			    admin_datetime = NOW(), processed_datetime = NOW(),
			    admin_users = 'system', version = version + 1
			WHERE sg_table = ? AND sg_id = ? AND processed = 0
		`, cand.Table, cand.SGID)
		if res.Error != nil {
			result.Errors++
			continue
		}
		if res.RowsAffected == 0 {
			// 이미 처리됨 — 이력 남기지 않음
			continue
		}
		result.DismissedRows += int(res.RowsAffected)
		result.DismissedKeys = append(result.DismissedKeys, fmt.Sprintf("%s:%d", cand.Table, cand.SGID))

		// 상태 변경 이력 (g5_singo_history)
		if err := db.Exec(`
			INSERT INTO g5_singo_history
				(sg_table, sg_id, sg_parent, prev_status, new_status, admin_id, admin_note, created_at)
			VALUES (?, ?, ?, 'monitoring', 'dismissed', 'system', ?, NOW())
		`, cand.Table, cand.SGID, cand.Parent, note).Error; err != nil {
			result.Errors++
		}
	}

	return result, nil
}
