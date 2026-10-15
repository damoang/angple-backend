package gnuboard

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/damoang/angple-backend/internal/domain/gnuboard"
	"gorm.io/gorm"
)

// 글 상세의 예약삭제 조회 게이트.
//
// 글 상세는 매 요청 예약삭제 행(pending)을 찾지만, pending 행이 있는 글은 드물다. 그래서 pending 인
// (bo_table, wr_id) 집합 전체를 프로세스에 scheduledPendingTTL 동안 들고, 집합에 있는 글만 FindByPost 로
// 행을 읽는다. 집합 조회가 실패하면 예전처럼 FindByPost 를 바로 부른다(fail-open).
//
// 이 저장소의 Create·Cancel·MarkExecuted·DeleteByPost 가 성공하면 집합을 비운다. 백엔드 프로세스가 여럿이면
// 다른 프로세스는 최대 scheduledPendingTTL 늦게 반영된다(글 상세의 예약삭제 안내만 해당).
// 수정·삭제·취소 같은 쓰기·권한 경로는 이 게이트를 쓰지 않고 FindByPost 로 직접 조회한다.

// scheduledPendingTTL 은 pending 집합 프로세스 캐시의 수명이다.
const scheduledPendingTTL = 20 * time.Second

// scheduledPendingLogInterval 은 집합 조회 실패 로그의 최소 간격이다(요청마다 찍히지 않게).
const scheduledPendingLogInterval = time.Minute

// scheduledPendingKey 는 pending 집합의 원소다.
type scheduledPendingKey struct {
	boTable string
	wrID    int
}

// scheduledPendingCache 는 pending 집합의 프로세스 캐시다. gen 은 무효화 세대 번호로, 조회 도중 무효화가
// 끼어들면 그 조회 결과는 넣지 않는다(무효화 전 상태가 다시 적재되지 않게).
type scheduledPendingCache struct {
	mu      sync.Mutex
	set     map[scheduledPendingKey]struct{}
	expires time.Time
	gen     uint64
}

var scheduledPending = &scheduledPendingCache{}

// scheduledPendingLastLog 는 마지막 실패 로그 시각(UnixNano)이다.
var scheduledPendingLastLog atomic.Int64

// scheduledPendingLoader 는 pending 집합을 DB 에서 읽는다. 테스트가 실패를 흉내 낼 때 바꿔 끼운다.
var scheduledPendingLoader = loadScheduledPendingSet

func (c *scheduledPendingCache) get(now time.Time) (map[scheduledPendingKey]struct{}, uint64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.set == nil || !now.Before(c.expires) {
		return nil, c.gen, false
	}
	return c.set, c.gen, true
}

func (c *scheduledPendingCache) put(set map[scheduledPendingKey]struct{}, gen uint64, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if gen != c.gen { // 조회 도중 무효화됐다 — 낡았을 수 있으니 넣지 않는다
		return
	}
	c.set = set
	c.expires = now.Add(scheduledPendingTTL)
}

func (c *scheduledPendingCache) invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gen++
	c.set = nil
}

// loadScheduledPendingSet 은 pending 인 (bo_table, wr_id) 를 전부 읽는다.
func loadScheduledPendingSet(db *gorm.DB) (map[scheduledPendingKey]struct{}, error) {
	var rows []struct {
		BoTable string `gorm:"column:bo_table"`
		WrID    int    `gorm:"column:wr_id"`
	}
	if err := db.Raw("SELECT bo_table, wr_id FROM g5_scheduled_deletes WHERE status = 'pending'").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	set := make(map[scheduledPendingKey]struct{}, len(rows))
	for _, r := range rows {
		set[scheduledPendingKey{boTable: r.BoTable, wrID: r.WrID}] = struct{}{}
	}
	return set, nil
}

// scheduledPendingSet 은 pending 집합을 돌려준다(프로세스 캐시 → 없으면 1쿼리). 조회 실패는 캐시하지 않는다.
func scheduledPendingSet(db *gorm.DB) (map[scheduledPendingKey]struct{}, error) {
	now := time.Now()
	set, gen, ok := scheduledPending.get(now)
	if ok {
		return set, nil
	}
	set, err := scheduledPendingLoader(db)
	if err != nil {
		return nil, err
	}
	scheduledPending.put(set, gen, now)
	return set, nil
}

// InvalidateScheduledPending 은 이 프로세스의 pending 집합 캐시를 비운다. 예약삭제 행을 바꾼 뒤 부른다
// (이 저장소의 쓰기 메서드는 성공 뒤 스스로 부른다).
func InvalidateScheduledPending() {
	scheduledPending.invalidate()
}

// FindByPostForView 는 글 상세 표시용 조회다. pending 집합에 없는 글은 DB 를 읽지 않고 not found 를 돌려준다.
// 집합에 있거나 집합 조회가 실패하면 FindByPost 와 같다.
func (r *scheduledDeleteRepository) FindByPostForView(boTable string, wrID int) (*gnuboard.ScheduledDelete, error) {
	set, err := scheduledPendingSet(r.db)
	if err != nil {
		now := time.Now().UnixNano()
		last := scheduledPendingLastLog.Load()
		if now-last >= int64(scheduledPendingLogInterval) && scheduledPendingLastLog.CompareAndSwap(last, now) {
			log.Printf("[scheduled_delete] pending 집합 조회 실패 — 단건 조회로 대신함: %v", err)
		}
		return r.FindByPost(boTable, wrID)
	}
	if _, ok := set[scheduledPendingKey{boTable: boTable, wrID: wrID}]; !ok {
		return nil, fmt.Errorf("scheduled delete not found: %w", gorm.ErrRecordNotFound)
	}
	return r.FindByPost(boTable, wrID)
}
