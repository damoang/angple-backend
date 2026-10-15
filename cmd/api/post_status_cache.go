package main

import (
	"log"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"
)

// 글 상세의 처리 상태(g5_da_post_status) 조회 게이트.
//
// 상태 행은 일부 게시판에만 있는데 글 상세는 매 요청 단건 조회를 했다. 그래서 게시판별
// wr_id → 상태 행 맵을 프로세스에 postStatusBoardTTL 동안 들고 상세는 맵에서 꺼낸다(게시판당 1쿼리,
// 단건 조회와 같은 컬럼·포맷 postStatusSelect). 맵 조회가 실패하면 캐시하지 않고 단건 조회로 대신한다.
//
// upsertPostStatus·clearPostStatus 가 성공하면 그 게시판 맵을 비운다. 백엔드 프로세스가 여럿이면 다른
// 프로세스는 최대 postStatusBoardTTL 늦게 반영된다(글 상세의 상태 배지만 해당).

// postStatusBoardTTL 은 게시판별 상태 맵 프로세스 캐시의 수명이다.
const postStatusBoardTTL = 60 * time.Second

// postStatusBoardMaxBoards 는 프로세스 캐시에 담는 게시판 수 상한이다. 넘으면 통째로 비운다.
const postStatusBoardMaxBoards = 1000

// postStatusLogInterval 은 맵 조회 실패 로그의 최소 간격이다(요청마다 찍히지 않게).
const postStatusLogInterval = time.Minute

// postStatusBoardEntry 는 한 게시판의 상태 맵과 만료 시각이다.
type postStatusBoardEntry struct {
	byID    map[int]postStatusRow
	expires time.Time
}

// postStatusBoardCache 는 게시판별 상태 맵의 프로세스 캐시다. gens 는 게시판별 무효화 세대 번호로,
// 조회 도중 무효화가 끼어들면 그 조회 결과는 넣지 않는다(무효화 전 상태가 다시 적재되지 않게).
type postStatusBoardCache struct {
	mu      sync.Mutex
	entries map[string]postStatusBoardEntry
	gens    map[string]uint64
}

var postStatusBoards = &postStatusBoardCache{
	entries: make(map[string]postStatusBoardEntry),
	gens:    make(map[string]uint64),
}

// postStatusLastLog 는 마지막 실패 로그 시각(UnixNano)이다.
var postStatusLastLog atomic.Int64

// postStatusBoardLoader 는 한 게시판의 상태 맵을 DB 에서 읽는다. 테스트가 바꿔 끼운다.
var postStatusBoardLoader = loadPostStatusBoard

func (c *postStatusBoardCache) get(slug string, now time.Time) (map[int]postStatusRow, uint64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	gen := c.gens[slug]
	e, ok := c.entries[slug]
	if !ok || !now.Before(e.expires) {
		return nil, gen, false
	}
	return e.byID, gen, true
}

func (c *postStatusBoardCache) put(slug string, byID map[int]postStatusRow, gen uint64, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gens[slug] != gen { // 조회 도중 무효화됐다 — 낡았을 수 있으니 넣지 않는다
		return
	}
	if len(c.entries) >= postStatusBoardMaxBoards {
		c.entries = make(map[string]postStatusBoardEntry)
	}
	c.entries[slug] = postStatusBoardEntry{byID: byID, expires: now.Add(postStatusBoardTTL)}
}

func (c *postStatusBoardCache) forget(slug string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gens[slug]++
	delete(c.entries, slug)
}

func (c *postStatusBoardCache) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]postStatusBoardEntry)
	c.gens = make(map[string]uint64)
}

// loadPostStatusBoard 는 한 게시판의 상태 행 전부를 읽는다(단건 조회와 같은 컬럼·포맷).
func loadPostStatusBoard(db *gorm.DB, slug string) (map[int]postStatusRow, error) {
	var rows []postStatusRow
	if err := db.Table(postStatusTable).
		Select(postStatusSelect).
		Where("board_id = ?", slug).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	byID := make(map[int]postStatusRow, len(rows))
	for _, r := range rows {
		byID[r.WrID] = r
	}
	return byID, nil
}

// postStatusBoardMap 은 한 게시판의 상태 맵을 돌려준다(프로세스 캐시 → 없으면 1쿼리). 조회 실패는 캐시하지 않는다.
func postStatusBoardMap(db *gorm.DB, slug string) (map[int]postStatusRow, error) {
	now := time.Now()
	byID, gen, ok := postStatusBoards.get(slug, now)
	if ok {
		return byID, nil
	}
	byID, err := postStatusBoardLoader(db, slug)
	if err != nil {
		return nil, err
	}
	postStatusBoards.put(slug, byID, gen, now)
	return byID, nil
}

// invalidatePostStatusBoard 는 이 프로세스의 그 게시판 상태 맵을 비운다. 상태 행을 바꾼 뒤 부른다.
func invalidatePostStatusBoard(slug string) {
	postStatusBoards.forget(slug)
}

// postStatusLogRateLimited 는 postStatusLogInterval 에 한 번만 로그를 남긴다.
func postStatusLogRateLimited(format string, args ...any) {
	now := time.Now().UnixNano()
	last := postStatusLastLog.Load()
	if now-last >= int64(postStatusLogInterval) && postStatusLastLog.CompareAndSwap(last, now) {
		log.Printf(format, args...)
	}
}
