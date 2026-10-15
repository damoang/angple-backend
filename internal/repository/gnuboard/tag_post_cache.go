package gnuboard

import (
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"
)

// 글 상세의 태그 조회 게이트.
//
// 글 상세는 매 요청 그 글의 태그(g5_na_tag_log)를 찾지만, 태그가 있는 글은 일부다. 그래서 태그가 하나라도 있는
// (bo_table, wr_id) 집합 전체를 프로세스에 taggedPostTTL 동안 들고, 집합에 있는 글만 GetPostTags 로 행을 읽는다.
// 집합 조회가 실패하면 예전처럼 GetPostTags 를 바로 부른다(fail-open).
//
// 키의 bo_table 은 소문자로, 끝 공백은 떼어 맞춘다(양쪽 모두). DB 비교는 대소문자를 가리지 않으므로, 게이트가
// 그보다 엄격해서 실제로 태그가 있는 글을 놓치는 일이 없게 한다.
//
// 이 저장소의 SetPostTags·DeletePostTags 가 성공하면 집합을 비운다. 이 저장소를 거치지 않은 태그 변경(다른
// 서비스의 자동 태그·해제 등)과 다른 백엔드 프로세스의 변경은 최대 taggedPostTTL 늦게 글 상세에 반영된다.
// 목록의 GetPostsTagsBatch 와 쓰기 경로는 이 게이트를 쓰지 않는다.

// taggedPostTTL 은 태그 있는 글 집합 프로세스 캐시의 수명이다.
const taggedPostTTL = 30 * time.Second

// taggedPostLogInterval 은 집합 조회 실패 로그의 최소 간격이다(요청마다 찍히지 않게).
const taggedPostLogInterval = time.Minute

// taggedPostKey 는 태그 있는 글 집합의 원소다. boTable 은 normalizeTagBoTable 을 거친 값이다.
type taggedPostKey struct {
	boTable string
	wrID    int
}

// taggedPostCache 는 태그 있는 글 집합의 프로세스 캐시다. gen 은 무효화 세대 번호로, 조회 도중 무효화가
// 끼어들면 그 조회 결과는 넣지 않는다(무효화 전 상태가 다시 적재되지 않게).
type taggedPostCache struct {
	mu      sync.Mutex
	set     map[taggedPostKey]struct{}
	expires time.Time
	gen     uint64
}

var taggedPosts = &taggedPostCache{}

// taggedPostLastLog 는 마지막 실패 로그 시각(UnixNano)이다.
var taggedPostLastLog atomic.Int64

// taggedPostLoader 는 태그 있는 글 집합을 DB 에서 읽는다. 테스트가 실패를 흉내 낼 때 바꿔 끼운다.
var taggedPostLoader = loadTaggedPostSet

func (c *taggedPostCache) get(now time.Time) (map[taggedPostKey]struct{}, uint64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.set == nil || !now.Before(c.expires) {
		return nil, c.gen, false
	}
	return c.set, c.gen, true
}

func (c *taggedPostCache) put(set map[taggedPostKey]struct{}, gen uint64, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if gen != c.gen { // 조회 도중 무효화됐다 — 낡았을 수 있으니 넣지 않는다
		return
	}
	c.set = set
	c.expires = now.Add(taggedPostTTL)
}

func (c *taggedPostCache) invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gen++
	c.set = nil
}

// normalizeTagBoTable 은 집합 키의 bo_table 을 DB 비교와 같은 기준(대소문자 무시·끝 공백 무시)으로 맞춘다.
func normalizeTagBoTable(boTable string) string {
	return strings.ToLower(strings.TrimRight(boTable, " "))
}

// loadTaggedPostSet 은 태그가 하나라도 있는 (bo_table, wr_id) 를 전부 읽는다.
func loadTaggedPostSet(db *gorm.DB) (map[taggedPostKey]struct{}, error) {
	var rows []struct {
		BoTable string `gorm:"column:bo_table"`
		WrID    int    `gorm:"column:wr_id"`
	}
	if err := db.Raw("SELECT DISTINCT bo_table, wr_id FROM g5_na_tag_log").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	set := make(map[taggedPostKey]struct{}, len(rows))
	for _, r := range rows {
		set[taggedPostKey{boTable: normalizeTagBoTable(r.BoTable), wrID: r.WrID}] = struct{}{}
	}
	return set, nil
}

// taggedPostSet 은 태그 있는 글 집합을 돌려준다(프로세스 캐시 → 없으면 1쿼리). 조회 실패는 캐시하지 않는다.
func taggedPostSet(db *gorm.DB) (map[taggedPostKey]struct{}, error) {
	now := time.Now()
	set, gen, ok := taggedPosts.get(now)
	if ok {
		return set, nil
	}
	set, err := taggedPostLoader(db)
	if err != nil {
		return nil, err
	}
	taggedPosts.put(set, gen, now)
	return set, nil
}

// InvalidateTaggedPosts 는 이 프로세스의 태그 있는 글 집합 캐시를 비운다. 글의 태그를 바꾼 뒤 부른다
// (이 저장소의 SetPostTags·DeletePostTags 는 성공 뒤 스스로 부른다).
func InvalidateTaggedPosts() {
	taggedPosts.invalidate()
}

// GetPostTagsForView 는 글 상세 표시용 태그 조회다. 태그 있는 글 집합에 없는 글은 DB 를 읽지 않고 nil 을
// 돌려준다(태그 없음과 같다). 집합에 있거나 집합 조회가 실패하면 GetPostTags 와 같다.
func (r *tagRepository) GetPostTagsForView(boTable string, wrID int) ([]string, error) {
	set, err := taggedPostSet(r.db)
	if err != nil {
		now := time.Now().UnixNano()
		last := taggedPostLastLog.Load()
		if now-last >= int64(taggedPostLogInterval) && taggedPostLastLog.CompareAndSwap(last, now) {
			log.Printf("[tag] 태그 있는 글 집합 조회 실패 — 단건 조회로 대신함: %v", err)
		}
		return r.GetPostTags(boTable, wrID)
	}
	if _, ok := set[taggedPostKey{boTable: normalizeTagBoTable(boTable), wrID: wrID}]; !ok {
		return nil, nil
	}
	return r.GetPostTags(boTable, wrID)
}
