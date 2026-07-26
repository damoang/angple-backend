package service

import (
	"encoding/json"
	"sync"
	"time"

	"gorm.io/gorm"
)

// GroupGlobalNotice 는 소모임(gr_id='group') 전 게시판 상단에 공통으로 노출할 공지 1건의 설정이다.
//
// 소모임마다 글을 복제하지 않고 원본 글 하나를 각 게시판 공지 응답에 끼워 넣는다.
// 복제하지 않는 이유는 댓글 때문이다 — 글이 91개로 갈라지면 의견도 91곳에 흩어진다.
//
// 설정은 g5_kv_store 에 두어 배포 없이 켜고 끌 수 있다.
//
//	`key`      system:group_global_notice
//	value_text {"board":"notice","wr_id":18300,"enabled":true}
type GroupGlobalNotice struct {
	Board   string `json:"board"`
	WrID    int    `json:"wr_id"`
	Enabled bool   `json:"enabled"`
}

const groupGlobalNoticeKey = "system:group_global_notice"

// 공지 응답은 Redis 에 2분 캐시되지만 캐시 미스마다 kv 를 읽게 되므로 짧게 메모이즈한다.
// 플래그를 꺼도 최대 (여기 60초 + 공지 캐시 2분) 뒤에는 반드시 사라진다.
const groupGlobalNoticeTTL = 60 * time.Second

var (
	ggnMu      sync.RWMutex
	ggnCached  *GroupGlobalNotice
	ggnFetched time.Time
)

// LoadGroupGlobalNotice 는 전역 공지 설정을 반환한다. 미설정·비활성·파싱 실패면 nil 이다.
//
// 설정이 깨져 있다고 공지 API 전체가 죽으면 안 되므로 모든 실패는 nil 로 떨어진다
// (= 전역 공지 기능만 꺼진 상태, 기존 게시판 공지는 그대로 동작).
func LoadGroupGlobalNotice(db *gorm.DB) *GroupGlobalNotice {
	ggnMu.RLock()
	if !ggnFetched.IsZero() && time.Since(ggnFetched) < groupGlobalNoticeTTL {
		cached := ggnCached
		ggnMu.RUnlock()
		return cached
	}
	ggnMu.RUnlock()

	var raw string
	// `key` 는 MySQL 예약어라 백틱이 필요하다.
	err := db.Table("g5_kv_store").
		Select("COALESCE(value_text, '')").
		Where("`key` = ?", groupGlobalNoticeKey).
		Scan(&raw).Error

	var cfg *GroupGlobalNotice
	if err == nil && raw != "" {
		var parsed GroupGlobalNotice
		if json.Unmarshal([]byte(raw), &parsed) == nil &&
			parsed.Enabled && parsed.Board != "" && parsed.WrID > 0 {
			cfg = &parsed
		}
	}

	ggnMu.Lock()
	ggnCached, ggnFetched = cfg, time.Now()
	ggnMu.Unlock()
	return cfg
}

// IsGroupBoard 는 slug 가 실제 존재하는 소모임 게시판인지 확인한다.
//
// 댓글의 유입 소모임은 클라이언트가 URL 파라미터로 보내는 값이라 위조가 가능하다.
// 반드시 이 화이트리스트를 통과한 값만 저장한다.
func IsGroupBoard(db *gorm.DB, slug string) bool {
	if slug == "" {
		return false
	}
	var n int64
	if err := db.Table("g5_board").
		Where("bo_table = ? AND gr_id = 'group'", slug).
		Count(&n).Error; err != nil {
		return false
	}
	return n > 0
}
