package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	"gorm.io/gorm"
)

// 글 처리 상태(해결됨·진행중·보류) — 카테고리(ca_name)와 **독립**인 표시.
//
// 버그 게시판은 지금까지 해결되면 ca_name 을 '완료' 로 덮어써 원래 종류(버그/기능제안)가
// 사라지고 「버그」 탭에서 해결 건이 빠졌다. 상태는 g5_da_post_status 에 따로 두고
// 목록·상세 응답에 status 로 실어 준다(설계: docs/2026-09-28-bug-status-badge-sprint.html).
//
// ⛔ 행이 없는 게시판은 응답 키가 생기지 않는다 — 다른 게시판 동작·응답 불변.
// ⛔ 조회 실패는 배지만 못 그릴 뿐 목록을 막지 않는다(로그 1줄).

const postStatusTable = "g5_da_post_status"

// postStatusAllowed 는 허용 상태값인지 본다.
func postStatusAllowed(status string) bool {
	switch status {
	case "resolved", "in_progress", "hold":
		return true
	}
	return false
}

type postStatusRow struct {
	WrID      int       `gorm:"column:wr_id"`
	Status    string    `gorm:"column:status"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

// enrichWithPostStatus 는 목록 항목에 status / status_updated_at 을 붙인다(페이지 글 묶음 1쿼리).
func enrichWithPostStatus(db *gorm.DB, slug string, items []map[string]any) []map[string]any {
	if db == nil || len(items) == 0 {
		return items
	}
	ids := make([]int, 0, len(items))
	for _, item := range items {
		if id := itemIntID(item); id > 0 {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return items
	}
	var rows []postStatusRow
	if err := db.Table(postStatusTable).
		Select("wr_id, status, updated_at").
		Where("board_id = ? AND wr_id IN ?", slug, ids).
		Find(&rows).Error; err != nil {
		// 테이블 부재(DDL 전 배포)·일시 오류 — 배지만 생략.
		log.Printf("[post_status] 목록 상태 조회 실패 board=%s: %v", slug, err)
		return items
	}
	if len(rows) == 0 {
		return items
	}
	byID := make(map[int]postStatusRow, len(rows))
	for _, r := range rows {
		byID[r.WrID] = r
	}
	for i, item := range items {
		if r, ok := byID[itemIntID(item)]; ok {
			items[i]["status"] = r.Status
			items[i]["status_updated_at"] = r.UpdatedAt.Format("2006-01-02 15:04:05")
		}
	}
	return items
}

// attachPostStatus 는 상세 응답 한 건에 status 를 붙인다.
func attachPostStatus(db *gorm.DB, slug string, id int, detail map[string]any) {
	if db == nil || detail == nil {
		return
	}
	var r postStatusRow
	err := db.Table(postStatusTable).
		Select("wr_id, status, updated_at").
		Where("board_id = ? AND wr_id = ?", slug, id).
		Take(&r).Error
	if err != nil {
		if err != gorm.ErrRecordNotFound {
			log.Printf("[post_status] 상세 상태 조회 실패 board=%s id=%d: %v", slug, id, err)
		}
		return
	}
	detail["status"] = r.Status
	detail["status_updated_at"] = r.UpdatedAt.Format("2006-01-02 15:04:05")
}

// upsertPostStatus 는 상태를 지정한다(누가·어떤 경로로 바꿨는지 함께 기록).
func upsertPostStatus(db *gorm.DB, slug string, id int, status, setBy, source, note string) error {
	if !postStatusAllowed(status) {
		return fmt.Errorf("invalid status: %s", status)
	}
	setBy = strings.TrimSpace(setBy)
	if len(setBy) > 20 {
		setBy = setBy[:20]
	}
	if len(note) > 255 {
		note = note[:255]
	}
	return db.Exec(
		"INSERT INTO "+postStatusTable+" (board_id, wr_id, status, set_by, source, note, created_at, updated_at) "+
			"VALUES (?, ?, ?, ?, ?, ?, NOW(3), NOW(3)) "+
			"ON DUPLICATE KEY UPDATE status = VALUES(status), set_by = VALUES(set_by), source = VALUES(source), note = VALUES(note), updated_at = NOW(3)",
		slug, id, status, setBy, source, note,
	).Error
}

// clearPostStatus 는 상태를 해제한다(행 삭제).
func clearPostStatus(db *gorm.DB, slug string, id int) error {
	return db.Exec("DELETE FROM "+postStatusTable+" WHERE board_id = ? AND wr_id = ?", slug, id).Error
}

// itemIntID 는 목록 항목의 id 를 int 로 읽는다(transform 이 int 로 넣지만 방어).
func itemIntID(item map[string]any) int {
	switch v := item["id"].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return 0
}
