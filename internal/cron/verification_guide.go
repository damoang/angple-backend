package cron

import (
	"fmt"
	"log"
	"math/rand"
	"time"

	"gorm.io/gorm"
)

// verificationBoard is the gnuboard slug for the 해외 실명인증 board.
const verificationBoard = "verification"

// VerificationGuideResult contains the result of the verification-guide cron.
type VerificationGuideResult struct {
	ExecutedAt string `json:"executed_at"`
	Processed  int    `json:"processed"`
	Errors     int    `json:"errors"`
	WrIDs      []int  `json:"wr_ids,omitempty"`
}

// runVerificationGuide scans new posts on the verification(해외 실명인증) board that
// have not yet been assigned a 난수(6-digit code), stores the code in wr_1, and posts
// an AI(다모앙) guide comment carrying that code.
//
// 멱등: 난수 부여 여부를 wr_1(빈값=미처리)로 판정하고, UPDATE는 wr_1 빈 경우만 적용해
// 동시 실행/재실행에도 중복 댓글이 생기지 않는다. 공지(bo_notice)·시스템계정 글은 제외.
// 비밀글이라 이 가이드 댓글은 작성자 본인+관리자만 열람한다.
func runVerificationGuide(db *gorm.DB) (*VerificationGuideResult, error) {
	now := time.Now()
	result := &VerificationGuideResult{ExecutedAt: now.Format("2006-01-02 15:04:05")}

	// 공지글(bo_notice)은 난수 부여 대상이 아니다(안내/운영 공지).
	var boNotice string
	db.Raw("SELECT bo_notice FROM g5_board WHERE bo_table = ?", verificationBoard).Scan(&boNotice)

	type target struct {
		WrID  int
		WrNum int
	}
	var targets []target
	q := db.Table("g5_write_" + verificationBoard).
		Select("wr_id, wr_num").
		Where("wr_is_comment = 0").
		Where("wr_deleted_at IS NULL").
		Where("(wr_1 IS NULL OR wr_1 = '')").
		Where("mb_id NOT IN ('ai','admin','police')")
	if boNotice != "" {
		q = q.Where("FIND_IN_SET(wr_id, ?) = 0", boNotice)
	}
	if err := q.Order("wr_id ASC").Find(&targets).Error; err != nil {
		return nil, fmt.Errorf("verification 대상 조회 실패: %w", err)
	}

	nowStr := now.Format("2006-01-02 15:04:05")

	for _, t := range targets {
		nansu := rand.Intn(900000) + 100000 // 6자리 (100000~999999)
		content := fmt.Sprintf(
			`<p>안녕하세요, 다모앙입니다. 🙇 해외 실명인증 신청 감사합니다.</p>`+
				`<p><b>[인증 난수] %d</b></p>`+
				`<p>확인 후 인증 처리해 드리겠습니다. 문의는 security@damoang.net 으로 회신 주세요.</p>`,
			nansu,
		)

		worked := false
		err := db.Transaction(func(tx *gorm.DB) error {
			// 1. 난수 저장 — wr_1 이 아직 빈 경우에만 (동시/재실행 중복 가드)
			res := tx.Exec(
				"UPDATE g5_write_verification SET wr_1 = ? WHERE wr_id = ? AND (wr_1 IS NULL OR wr_1 = '')",
				fmt.Sprintf("%d", nansu), t.WrID,
			)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return nil // 이미 다른 실행이 처리함 → 댓글도 달지 않고 skip
			}
			worked = true

			// 2. 이 원글의 다음 wr_comment 값
			var nextComment int
			tx.Raw(
				"SELECT COALESCE(MAX(wr_comment), -1) + 1 FROM g5_write_verification WHERE wr_parent = ? AND wr_is_comment = 1",
				t.WrID,
			).Scan(&nextComment)

			// 3. AI(다모앙) 가이드 댓글 INSERT
			if err := tx.Exec(`
				INSERT INTO g5_write_verification
				(wr_num, wr_reply, wr_parent, wr_is_comment, wr_comment, wr_comment_reply,
				 wr_subject, wr_content, wr_name, mb_id, wr_password, wr_email, wr_homepage,
				 wr_link1, wr_link2, wr_hit, wr_good, wr_nogood, wr_option, wr_datetime, wr_last, wr_ip)
				VALUES (?, '', ?, 1, ?, '', '', ?, '다모앙', 'ai', '', '', '', '', '', 0, 0, 0, 'html1', ?, '', '9.9.9.9')`,
				t.WrNum, t.WrID, nextComment, content, nowStr,
			).Error; err != nil {
				return err
			}

			// 4. 원글 댓글수 +1
			return tx.Exec("UPDATE g5_write_verification SET wr_comment = wr_comment + 1 WHERE wr_id = ?", t.WrID).Error
		})
		if err != nil {
			result.Errors++
			log.Printf("[Cron:verification-guide] wr_id=%d error: %v", t.WrID, err)
			continue
		}
		if worked {
			result.Processed++
			result.WrIDs = append(result.WrIDs, t.WrID)
		}
	}

	return result, nil
}
