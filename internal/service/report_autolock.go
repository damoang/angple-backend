package service

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"

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

// reportWindowDays 는 자동 잠금 집계에 포함할 신고의 기간이다.
// 기간 제한이 없으면 오래 전 신고가 계속 누적되어, 시간이 지나기만 해도
// 임계에 도달하는 한 방향 래칫이 된다.
const reportWindowDays = 7

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

	// 고유 신고자 수. 취소된 신고(sg_flag != 0)와 기간 밖 신고는 제외한다.
	var reporters int64
	if err := db.Raw(`
		SELECT COUNT(DISTINCT mb_id) FROM g5_na_singo
		 WHERE sg_table = ? AND sg_id = ? AND sg_parent = ? AND sg_flag = 0
		   AND sg_time >= DATE_SUB(NOW(), INTERVAL ? DAY)
	`, boTable, sgID, sgParent, reportWindowDays).Scan(&reporters).Error; err != nil {
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

	// 작성자 냉각(임시 제한) 발행. 격해진 순간에 이어 쓰는 것을 잠시 막는다.
	// ⛔제재가 아니므로 징계 기록·사다리에 올리지 않으며, 만료 시각으로 자동 해제된다.
	// 설정이 비활성(0분)이면 아무 일도 하지 않는다.
	var author string
	if err := db.Raw(
		fmt.Sprintf("SELECT mb_id FROM `%s` WHERE wr_id = ? AND wr_is_comment = ?", writeTable),
		sgID, commentFlag,
	).Scan(&author).Error; err != nil {
		log.Printf("[autolock] 작성자 조회 실패 (%s/%d): %v", boTable, sgID, err)
	} else {
		ApplyReportFreeze(db, author, fmt.Sprintf("신고 누적 잠금 %s/%d", boTable, sgID))
	}

	// 잠긴 글을 진실의 방에서 볼 수 있게 참조글을 만든다.
	// 참조글이 없으면 잠금 안내에서 진실의 방으로 가는 링크가 생성되지 않는다.
	if !isComment {
		createTruthroomReference(db, boTable, sgID)
	}
}

// htmlTagPattern 은 미리보기 생성 시 태그를 제거하기 위한 패턴이다.
var htmlTagPattern = regexp.MustCompile(`<[^>]*>`)

// createTruthroomReference 는 잠긴 글에 대한 진실의 방 참조글을 만든다.
//
// 원본은 옮기지 않고 제자리에 둔다. 참조만 만들기 때문에 되돌리기가 단순하고,
// 신고 레코드(g5_na_singo)의 대상 보드도 바뀌지 않는다.
//
// 프론트는 wr_1(보드), wr_2(글 ID), wr_3(빈 값) 으로 참조글을 찾으므로
// 이 세 필드를 규약대로 채운다.
func createTruthroomReference(db *gorm.DB, boTable string, postID int) {
	postIDStr := strconv.Itoa(postID)

	var existing int64
	if err := db.Raw(`
		SELECT COUNT(*) FROM g5_write_truthroom
		 WHERE wr_1 = ? AND wr_2 = ? AND wr_is_comment = 0
	`, boTable, postIDStr).Scan(&existing).Error; err != nil {
		log.Printf("[autolock] 진실의 방 중복 확인 실패 (%s/%d): %v", boTable, postID, err)
		return
	}
	if existing > 0 {
		return
	}

	writeTable := fmt.Sprintf("g5_write_%s", boTable)
	var original struct {
		WrSubject string `gorm:"column:wr_subject"`
		WrContent string `gorm:"column:wr_content"`
		MbID      string `gorm:"column:mb_id"`
		WrName    string `gorm:"column:wr_name"`
	}
	if err := db.Table(writeTable).
		Select("wr_subject, wr_content, mb_id, wr_name").
		Where("wr_id = ? AND wr_is_comment = 0", postID).
		First(&original).Error; err != nil {
		log.Printf("[autolock] 원본 조회 실패 (%s/%d): %v", boTable, postID, err)
		return
	}

	authorName := original.WrName
	if authorName == "" {
		authorName = original.MbID
	}

	preview := strings.TrimSpace(htmlTagPattern.ReplaceAllString(original.WrContent, " "))
	preview = strings.Join(strings.Fields(preview), " ")
	if runes := []rune(preview); len(runes) > 200 {
		preview = string(runes[:200]) + "…"
	}

	subject := fmt.Sprintf("[신고잠금] %s", original.WrSubject)
	content := fmt.Sprintf(
		`<div class="truthroom-preview"><p class="preview-text">%s</p>`+
			`<p class="preview-source">출처: <a href="/%s/%d">%s #%d</a></p></div>`,
		preview, boTable, postID, boTable, postID)
	link := fmt.Sprintf("https://damoang.net/%s/%d", boTable, postID)

	if err := db.Exec(`
		INSERT INTO g5_write_truthroom
		SET wr_id = (SELECT COALESCE(MAX(wr_id), 0) + 1 FROM g5_write_truthroom tmp),
			wr_num = (SELECT COALESCE(MIN(wr_num), 0) - 1 FROM g5_write_truthroom tmp2),
			wr_reply = '', wr_parent = 0, wr_is_comment = 0, wr_comment = 0,
			wr_comment_reply = '', ca_name = '게시글', wr_option = 'html1',
			wr_subject = ?, wr_content = ?, wr_link1 = ?, wr_link2 = '',
			wr_link1_hit = 0, wr_link2_hit = 0, wr_hit = 0, wr_good = 0, wr_nogood = 0,
			mb_id = ?, wr_password = '', wr_name = ?, wr_email = '', wr_homepage = '',
			wr_datetime = NOW(), wr_file = 0, wr_last = NOW(), wr_ip = '127.0.0.1',
			wr_1 = ?, wr_2 = ?,
			wr_3 = '', wr_4 = '', wr_5 = '',
			wr_6 = '', wr_7 = '', wr_8 = '', wr_9 = '', wr_10 = ''
	`, subject, content, link, original.MbID, authorName, boTable, postIDStr).Error; err != nil {
		log.Printf("[autolock] 진실의 방 참조글 생성 실패 (%s/%d): %v", boTable, postID, err)
		return
	}

	// wr_parent 는 자기 자신을 가리켜야 한다.
	db.Exec(`UPDATE g5_write_truthroom SET wr_parent = wr_id WHERE wr_parent = 0 AND wr_1 = ? AND wr_2 = ?`,
		boTable, postIDStr)
	db.Exec("UPDATE g5_board SET bo_count_write = bo_count_write + 1 WHERE bo_table = 'truthroom'")

	log.Printf("[autolock] created truthroom reference for %s/%d", boTable, postID)
}
