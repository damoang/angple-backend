package cron

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	"gorm.io/gorm"
)

// safeBoardTable 는 g5_board.bo_table 값이 동적 테이블명으로 안전한지 검증한다(SQL injection 방지).
var safeBoardTable = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// WithdrawalGraceResult 는 숙려기간 확정 익명화 cron 실행 결과다.
type WithdrawalGraceResult struct {
	CandidateCount  int      `json:"candidate_count"`  // 숙려 경과 후보 수
	AnonymizedCount int      `json:"anonymized_count"` // 이번 실행에서 익명화한 수
	SkippedCount    int      `json:"skipped_count"`    // 이미 익명화되어 스킵한 수(멱등)
	PostsUpdated    int      `json:"posts_updated"`    // 작성자명/본문 익명화된 게시물 행 수
	Errors          int      `json:"errors"`
	AnonymizedIDs   []string `json:"anonymized_ids"`
	ExecutedAt      string   `json:"executed_at"`
}

// withdrawalCandidate 는 g5_member 후보 행이다.
type withdrawalCandidate struct {
	MbNo        int    `gorm:"column:mb_no"`
	MbID        string `gorm:"column:mb_id"`
	MbNick      string `gorm:"column:mb_nick"`
	MbLeaveDate string `gorm:"column:mb_leave_date"`
}

// runWithdrawalGraceAnonymize 는 숙려기간(30일)이 경과한 탈퇴 신청 계정을 확정 익명화한다.
//
// 동작 원칙(반드시 유지):
//   - admin AnonymizeMember(anonymizeMemberUser)와 동일하게 "닉네임만" 익명화하고 행은 보존한다.
//   - DI(mb_dupinfo)·가입IP(mb_ip)·oauth uid·mb_intercept_date·mb_leave_date 등 다중이/제재 판별
//     식별자는 절대 삭제·변경하지 않는다(하드삭제 로직 추가 금지).
//   - 멱등: 이미 익명화(mb_nick 이 익명 접두)된 계정은 스킵한다. 2회 실행해도 무해.
//
// cron 은 특정 게시물 target 이 없으므로(신원 익명화 목적) target 기반 본문 치환은 수행하지 않는다.
func runWithdrawalGraceAnonymize(db *gorm.DB) (*WithdrawalGraceResult, error) {
	now := time.Now()
	result := &WithdrawalGraceResult{ExecutedAt: now.Format("2006-01-02 15:04:05")}

	// 후보: mb_leave_date 세팅됨. 정확한 30일 경과/형식 판정은 Go 에서 ClassifyWithdrawal 로 수행
	// (discipline-release 와 동일하게 형식 혼재를 Go 파싱으로 방어).
	var candidates []withdrawalCandidate
	if err := db.Table("g5_member").
		Select("mb_no, mb_id, mb_nick, mb_leave_date").
		Where("mb_leave_date != '' AND mb_leave_date IS NOT NULL").
		Find(&candidates).Error; err != nil {
		return nil, err
	}

	for _, cand := range candidates {
		state, _ := common.ClassifyWithdrawal(cand.MbLeaveDate, now)
		if state != common.WithdrawalConfirmed {
			continue // 아직 숙려중 → 대상 아님
		}
		result.CandidateCount++

		// 멱등: 이미 익명화된 계정은 스킵.
		if common.IsWithdrawalAnonymized(cand.MbNick) {
			result.SkippedCount++
			continue
		}

		posts, err := anonymizeWithdrawnMember(db, cand)
		if err != nil {
			log.Printf("[Cron:withdrawal-grace] anonymize failed for %s: %v", cand.MbID, err)
			result.Errors++
			continue
		}
		result.AnonymizedCount++
		result.PostsUpdated += posts
		result.AnonymizedIDs = append(result.AnonymizedIDs, cand.MbID)
	}

	return result, nil
}

// anonymizeWithdrawnMember 는 한 회원의 신원을 익명화한다(닉네임 + 과거 게시물 작성자명/본문 내 닉).
// 행은 보존하며 DI(mb_dupinfo)·IP·mb_intercept_date·mb_leave_date 등 식별자는 변경/삭제하지 않는다.
// 반환값은 익명화된 게시물 행 수.
//
// 순서가 중요하다: 게시물 치환을 먼저 하고(구 닉 필요), **모든 대상 게시판 치환이 성공했을 때만**
// 닉 마커를 세운다. 하나라도 실패하면 마커를 세우지 않아(에러 반환) 다음 cron 에서 재시도되며,
// 이미 치환된 글은 REPLACE/조건 no-op 이라 멱등 재실행으로 안전하게 수렴한다.
func anonymizeWithdrawnMember(db *gorm.DB, cand withdrawalCandidate) (int, error) {
	oldNick := strings.TrimSpace(cand.MbNick)
	// mb_no 로 결정론적 익명 닉을 만들어 충돌 없이 멱등 판정이 가능하도록 한다.
	replacement := fmt.Sprintf("%s회원_%d", common.WithdrawalAnonymizedNickPrefix, cand.MbNo)

	// 1. 과거 게시물의 작성자명(wr_name) 및 본문/제목 내 옛 닉(PII)을 일괄 치환한다(본인 글 한정).
	//    부분 실패 시 err != nil → 아래 닉 마커를 세우지 않고 반환(재시도 보장).
	posts, failures, err := anonymizeMemberPosts(db, cand.MbID, oldNick, replacement)
	if err != nil {
		log.Printf("[Cron:withdrawal-grace] posts anonymize incomplete for %s, marker NOT set (will retry): %v", cand.MbID, failures)
		return 0, err
	}

	// 2. 닉 마커 세팅 (g5_member + v2_users). 이 시점부터 해당 회원은 멱등 스킵 대상이 된다.
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Table("g5_member").Where("mb_id = ?", cand.MbID).
			Updates(map[string]any{
				"mb_nick":      replacement,
				"mb_nick_date": time.Now().Format("2006-01-02"),
			}).Error; err != nil {
			return err
		}
		// v2_users 미러 닉네임 동기화 (있으면). username = mb_id.
		return tx.Table("v2_users").Where("username = ?", cand.MbID).
			Update("nickname", replacement).Error
	}); err != nil {
		return 0, err
	}
	return posts, nil
}

// anonymizeMemberPosts 는 회원(mbID)이 작성한 게시물의 작성자명(wr_name)과 본문/제목 내 옛 닉을
// 모든 게시판(g5_write_{bo_table})에서 치환한다. admin anonymizeMemberUser 의 게시물 익명화와
// 동등하되, target URL 큐레이션 대신 "작성자 기준 일괄" 경로를 쓴다(개인정보 파기 완전성).
// 삭제는 하지 않으며 wr_name/wr_subject/wr_content 만 갱신한다.
//
// 개별 테이블 UPDATE 실패는 집계·전파한다: 하나라도 실패하면 err != nil 을 반환하여 상위에서
// 닉 마커를 세우지 않게 한다(실패 게시판의 PII 영구 잔존 방지 — 다음 cron 에서 재시도).
// 반환: (치환된 행 수, 실패 게시판/사유 목록, 에러).
func anonymizeMemberPosts(db *gorm.DB, mbID, oldNick, replacement string) (int, []string, error) {
	var boTables []string
	if err := db.Table("g5_board").Pluck("bo_table", &boTables).Error; err != nil {
		// 게시판 목록을 못 읽으면 치환 완전성을 보장할 수 없으므로 하드 에러(닉 마킹 보류 → 재시도).
		return 0, nil, err
	}

	updated := 0
	var failures []string
	for _, bt := range boTables {
		bt = strings.TrimSpace(bt)
		if bt == "" || !safeBoardTable.MatchString(bt) {
			continue
		}
		table := "g5_write_" + bt

		// 작성자명(wr_name) 익명화 — 본인 글만.
		res := db.Table(table).Where("mb_id = ? AND wr_name != ?", mbID, replacement).
			Update("wr_name", replacement)
		if res.Error != nil {
			log.Printf("[Cron:withdrawal-grace] wr_name update FAILED on %s (%s): %v", table, mbID, res.Error)
			failures = append(failures, fmt.Sprintf("%s: %v", table, res.Error))
			continue
		}
		updated += int(res.RowsAffected)

		// 본문/제목 내 옛 닉 치환 — 본인 글만.
		if oldNick != "" && oldNick != replacement {
			cres := db.Exec("UPDATE "+table+
				" SET wr_subject = REPLACE(wr_subject, ?, ?), wr_content = REPLACE(wr_content, ?, ?)"+
				" WHERE mb_id = ? AND (wr_subject LIKE ? OR wr_content LIKE ?)",
				oldNick, replacement, oldNick, replacement, mbID, "%"+oldNick+"%", "%"+oldNick+"%")
			if cres.Error != nil {
				log.Printf("[Cron:withdrawal-grace] content replace FAILED on %s (%s): %v", table, mbID, cres.Error)
				failures = append(failures, fmt.Sprintf("%s(content): %v", table, cres.Error))
			}
		}
	}
	if len(failures) > 0 {
		return updated, failures, fmt.Errorf("post anonymization incomplete: %d board(s) failed", len(failures))
	}
	return updated, nil, nil
}
