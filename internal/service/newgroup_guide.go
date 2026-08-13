// Package service — newgroup(소모임 개설 신청) 안내 자동 댓글.
package service

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// newgroupGuideText 는 개설 신청 글에 다는 안내 문구다. (newgroup/1193 후속)
const newgroupGuideText = `안녕하세요, 다모앙 안내 AI입니다. 🙇

이 신청 글에 <b>공감 100개가 모이면 소모임 개설이 검토</b>됩니다.
함께하고 싶은 앙님들께 공감을 부탁드려 보세요!

※ 이 댓글은 자동으로 등록되었습니다.`

// kstNow 는 게시판 테이블(naive KST datetime) 규약에 맞춘 현재 시각 문자열이다.
// ⛔ DB 세션은 UTC 다 — NOW() 를 쓰면 9시간 어긋난다.
func kstNow() string {
	kst := time.FixedZone("KST", 9*60*60)
	return time.Now().In(kst).Format("2006-01-02 15:04:05")
}

// PostNewgroupGuideComment 는 newgroup 새 글(신청)에 안내 댓글을 단다.
//
// 멱등: 같은 글에 ai 계정 댓글이 이미 있으면 아무것도 하지 않는다 —
// write_after 이벤트 재시도·중복 발행에 안전하다.
// 공지 등 신청이 아닌 글을 거르는 판단은 하지 않는다(신청 카테고리 여부는
// 작성 시점에 알 수 없는 경우가 있어, newgroup 의 원글 전체를 대상으로 한다).
func PostNewgroupGuideComment(db *gorm.DB, wrID int) error {
	// 원글 확인 (댓글·삭제글 제외)
	var post struct {
		WrID      int    `gorm:"column:wr_id"`
		WrNum     int    `gorm:"column:wr_num"`
		CaName    string `gorm:"column:ca_name"`
		WrComment int    `gorm:"column:wr_comment"`
	}
	err := db.Table("g5_write_newgroup").
		Select("wr_id, wr_num, ca_name, wr_comment").
		Where("wr_id = ? AND wr_is_comment = 0 AND wr_deleted_at IS NULL", wrID).
		Take(&post).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil // 이미 삭제됐거나 댓글 — 안내 대상 아님
		}
		return fmt.Errorf("newgroup guide: 원글 조회 실패: %w", err)
	}

	// 공지는 제외 — 운영 공지에 안내 댓글이 붙으면 소음이다.
	if post.CaName == "공지" {
		return nil
	}

	// 멱등 체크
	var exists int64
	if err := db.Table("g5_write_newgroup").
		Where("wr_parent = ? AND wr_is_comment = 1 AND mb_id = 'ai'", wrID).
		Count(&exists).Error; err != nil {
		return fmt.Errorf("newgroup guide: 멱등 체크 실패: %w", err)
	}
	if exists > 0 {
		return nil
	}

	now := kstNow()
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(
			`INSERT INTO g5_write_newgroup
			   (wr_num, wr_reply, wr_comment_reply, wr_parent, wr_is_comment, wr_comment,
			    ca_name, wr_option, wr_subject, wr_content, wr_seo_title,
			    wr_link1, wr_link2, wr_link1_hit, wr_link2_hit,
			    wr_hit, wr_good, wr_nogood, mb_id, wr_password, wr_name, wr_email, wr_homepage,
			    wr_datetime, wr_file, wr_last, wr_ip, wr_facebook_user, wr_twitter_user,
			    wr_1, wr_2, wr_3, wr_4, wr_5, wr_6, wr_7, wr_8, wr_9, wr_10)
			 VALUES (?, '', '', ?, 1, 0,
			    '', 'html1', '', ?, '',
			    '', '', 0, 0,
			    0, 0, 0, 'ai', '', '안내 AI', '', '',
			    ?, 0, ?, '127.0.0.1', '', '',
			    '', '', '', '', '', '', '', '', '', '')`,
			post.WrNum, wrID, newgroupGuideText, now, now,
		).Error; err != nil {
			return err
		}
		return tx.Exec(
			`UPDATE g5_write_newgroup SET wr_comment = wr_comment + 1 WHERE wr_id = ?`, wrID,
		).Error
	})
}
