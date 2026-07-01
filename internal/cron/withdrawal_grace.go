package cron

import (
	"fmt"
	"log"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	"gorm.io/gorm"
)

// WithdrawalGraceResult 는 숙려기간 확정 익명화 cron 실행 결과다.
type WithdrawalGraceResult struct {
	CandidateCount  int      `json:"candidate_count"`  // 숙려 경과 후보 수
	AnonymizedCount int      `json:"anonymized_count"` // 이번 실행에서 익명화한 수
	SkippedCount    int      `json:"skipped_count"`    // 이미 익명화되어 스킵한 수(멱등)
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

		if err := anonymizeWithdrawnMember(db, cand); err != nil {
			log.Printf("[Cron:withdrawal-grace] anonymize failed for %s: %v", cand.MbID, err)
			result.Errors++
			continue
		}
		result.AnonymizedCount++
		result.AnonymizedIDs = append(result.AnonymizedIDs, cand.MbID)
	}

	return result, nil
}

// anonymizeWithdrawnMember 는 한 회원의 신원(닉네임)만 익명화한다.
// v2_users.nickname 과 g5_member.mb_nick(+mb_nick_date)만 갱신하고, 그 외 식별자는 보존한다.
// 삭제(DELETE)나 DI/IP/intercept/leave_date 변경은 수행하지 않는다.
func anonymizeWithdrawnMember(db *gorm.DB, cand withdrawalCandidate) error {
	// mb_no 로 결정론적 익명 닉을 만들어 충돌 없이 멱등 판정이 가능하도록 한다.
	replacement := fmt.Sprintf("%s회원_%d", common.WithdrawalAnonymizedNickPrefix, cand.MbNo)

	return db.Transaction(func(tx *gorm.DB) error {
		// g5_member 닉네임 익명화 (행 보존, 그 외 컬럼 미변경)
		if err := tx.Table("g5_member").Where("mb_id = ?", cand.MbID).
			Updates(map[string]any{
				"mb_nick":      replacement,
				"mb_nick_date": time.Now().Format("2006-01-02"),
			}).Error; err != nil {
			return err
		}
		// v2_users 미러 닉네임 동기화 (있으면). username = mb_id.
		if err := tx.Table("v2_users").Where("username = ?", cand.MbID).
			Update("nickname", replacement).Error; err != nil {
			return err
		}
		return nil
	})
}
