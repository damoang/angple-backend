package service

import (
	"fmt"
	"log"
	"regexp"

	"gorm.io/gorm"
)

// 신고 누적 자동 잠금.
//
// 배경: 잠금 판정 로직은 존재했으나 신고 접수 경로가 이를 호출하지 않아
// 기능이 사실상 동작하지 않았다. 실제로는 운영진이 의견을 제출할 때만
// 평가되어, 아무도 의견을 달지 않으면 신고가 몇 건 쌓여도 잠기지 않았다.
// (2026-07-21 실측: 고유 신고자 59명인 글이 미잠금 상태로 방치)
//
// 설계 원칙:
//   - 판정은 한 곳에서만 한다. 신고를 받는 이 지점이 판정을 소유한다.
//   - 자동 조치는 되돌릴 수 있는 것까지만 한다. 여기서는 wr_7='lock' 만 세팅하며,
//     진실의 방 이동·wr_option='secret'·작성자 잠금은 하지 않는다.
//     되돌리기는 wr_7='' 한 줄이다.
//   - 임계값은 DB(g5_kv_store)에서 읽는다. 재배포 없이 조정하거나 0으로 비활성화할 수 있다.

// boardTablePattern 은 동적 테이블명에 쓸 수 있는 보드 slug 형식이다.
var boardTablePattern = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// ReportLockThreshold 는 자동 잠금 임계값(고유 신고자 수)을 반환한다.
// 값이 없거나 0 이하이면 0 을 반환하며, 이 경우 자동 잠금은 수행하지 않는다.
func ReportLockThreshold(db *gorm.DB) int {
	var result struct {
		ValueInt int64 `gorm:"column:value_int"`
	}
	err := db.Raw(
		"SELECT value_int FROM g5_kv_store WHERE `key` = 'system:report_lock_threshold' LIMIT 1",
	).Scan(&result).Error
	if err != nil || result.ValueInt <= 0 {
		return 0
	}
	return int(result.ValueInt)
}

// ApplyReportAutoLock 은 신고 접수 직후 호출되어, 해당 콘텐츠의 고유 신고자 수가
// 임계값 이상이면 wr_7 = 'lock' 을 세팅한다.
//
// 게시글 신고는 sgID == sgParent, 댓글 신고는 sgID != sgParent 로 구분한다.
// 게시글과 댓글은 각각 자기 자신에 대한 신고만 집계한다. 댓글 신고를 부모 글에
// 합산하면 본문 신고가 없는 글이 댓글 신고만으로 잠기기 때문이다.
//
// 이미 잠긴 콘텐츠는 조기 반환한다. 실패는 신고 접수 자체를 실패시키지 않으며,
// 로그만 남긴다.
func ApplyReportAutoLock(db *gorm.DB, boTable string, sgID, sgParent int) {
	threshold := ReportLockThreshold(db)
	if threshold <= 0 {
		return
	}
	if !boardTablePattern.MatchString(boTable) {
		log.Printf("[autolock] 잘못된 보드 테이블명: %q", boTable)
		return
	}

	isComment := sgParent > 0 && sgParent != sgID
	writeTable := fmt.Sprintf("g5_write_%s", boTable)

	commentFlag := 0
	if isComment {
		commentFlag = 1
	}

	// 이미 잠금이면 재작업하지 않는다.
	var currentWr7 string
	if err := db.Raw(
		fmt.Sprintf("SELECT IFNULL(wr_7, '') FROM `%s` WHERE wr_id = ? AND wr_is_comment = ?", writeTable),
		sgID, commentFlag,
	).Scan(&currentWr7).Error; err != nil {
		log.Printf("[autolock] wr_7 조회 실패 (%s/%d): %v", boTable, sgID, err)
		return
	}
	if currentWr7 == "lock" {
		return
	}

	// 고유 신고자 수. 취소된 신고(sg_flag != 0)는 제외한다.
	var reporters int64
	if err := db.Raw(`
		SELECT COUNT(DISTINCT mb_id) FROM g5_na_singo
		 WHERE sg_table = ? AND sg_id = ? AND sg_parent = ? AND sg_flag = 0
	`, boTable, sgID, sgParent).Scan(&reporters).Error; err != nil {
		log.Printf("[autolock] 신고자 집계 실패 (%s/%d): %v", boTable, sgID, err)
		return
	}
	if reporters < int64(threshold) {
		return
	}

	if err := db.Exec(
		fmt.Sprintf("UPDATE `%s` SET wr_7 = 'lock' WHERE wr_id = ? AND wr_is_comment = ?", writeTable),
		sgID, commentFlag,
	).Error; err != nil {
		log.Printf("[autolock] 잠금 실패 (%s/%d): %v", boTable, sgID, err)
		return
	}

	// 최신글 목록 동기화. 실패해도 잠금 자체는 유효하므로 로그만 남긴다.
	if !isComment {
		if err := db.Exec(
			"UPDATE g5_board_new SET wr_singo = 'lock' WHERE bo_table = ? AND wr_id = ?",
			boTable, sgID,
		).Error; err != nil {
			log.Printf("[autolock] board_new 동기화 실패 (%s/%d): %v", boTable, sgID, err)
		}
	}

	kind := "post"
	if isComment {
		kind = "comment"
	}
	log.Printf("[autolock] locked %s %s/%d (reporters: %d >= threshold: %d)",
		kind, boTable, sgID, reporters, threshold)
}
