package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// 보는 사람별 「메모 대상 ID 집합」 캐시다.
//
// 목록·글·댓글 응답에 author_memo 를 붙일 때, 보는 사람이 메모를 남긴 상대 ID 집합을
// 먼저 읽고 페이지 작성자와 겹칠 때만 메모 본문을 조회하게 하는 게이트다.
//
//   - 키는 보는 사람 ID 로만 만든다(memo:targets:{viewer}). 공유 캐시(목록·글·댓글)에는
//     이 집합도, 메모 본문도 들어가지 않는다.
//   - 값은 상대 ID 의 JSON 배열이다. 빈 배열은 「메모 없음」이다.
//   - 상한(MemoTargetsLimit)을 넘는 보는 사람은 캐시하지 않는다 — 호출자는 전체 조회로 간다.
//   - Redis 오류·시간 초과·역직렬화 실패는 「모름」으로 돌려준다(메모 없음으로 간주하지 않는다).
//   - 메모 쓰기 성공 뒤 Invalidate 로 그 보는 사람 키를 지운다. 지우기 실패는 TTL 이 상한이다.

// MemoTargetsCacheTTL 은 보는 사람별 메모 대상 집합 캐시의 수명이다.
const MemoTargetsCacheTTL = 10 * time.Minute

// memoTargetsCacheKeyPrefix 는 메모 대상 집합 캐시 키 접두사다 — `memo:targets:{viewerID}`.
const memoTargetsCacheKeyPrefix = "memo:targets:"

// 메모 대상 집합 캐시 Redis 호출 시간 상한. 읽기는 요청 경로라 짧게, 넘기면 전체 조회로 간다.
const (
	memoTargetsCacheReadTimeout  = 100 * time.Millisecond
	memoTargetsCacheWriteTimeout = 200 * time.Millisecond
	memoTargetsCacheDelTimeout   = 500 * time.Millisecond
)

// memoTargetsCacheLogInterval 은 캐시 실패 로그의 최소 간격이다(요청마다 찍히지 않게).
const memoTargetsCacheLogInterval = time.Minute

// memoTargetsCacheLastLog 는 마지막 실패 로그 시각(UnixNano)이다.
var memoTargetsCacheLastLog atomic.Int64

func logMemoTargetsCache(format string, args ...any) {
	now := time.Now().UnixNano()
	last := memoTargetsCacheLastLog.Load()
	if now-last < int64(memoTargetsCacheLogInterval) {
		return
	}
	if memoTargetsCacheLastLog.CompareAndSwap(last, now) {
		log.Printf("[memo-targets-cache] "+format, args...)
	}
}

// MemoTargetsStore 는 메모 대상 집합 캐시가 쓰는 Redis 명령의 최소 집합이다(*redis.Client 가 만족한다).
type MemoTargetsStore interface {
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
}

// MemoTargetsCache 는 보는 사람별 메모 대상 ID 집합 캐시다. nil 이면 모든 메서드가 「모름」·무동작이다.
type MemoTargetsCache struct {
	store MemoTargetsStore
}

// NewMemoTargetsCache 는 Redis 클라이언트로 캐시를 만든다. rdb 가 nil 이면 nil 을 돌려준다.
func NewMemoTargetsCache(rdb *redis.Client) *MemoTargetsCache {
	if rdb == nil {
		return nil
	}
	return &MemoTargetsCache{store: rdb}
}

// NewMemoTargetsCacheWithStore 는 임의 저장소로 캐시를 만든다(테스트용). store 가 nil 이면 nil 을 돌려준다.
func NewMemoTargetsCacheWithStore(store MemoTargetsStore) *MemoTargetsCache {
	if store == nil {
		return nil
	}
	return &MemoTargetsCache{store: store}
}

// MemoTargetsCacheKey 는 보는 사람의 메모 대상 집합 캐시 키다.
//
// member_id 비교는 DB 에서 대소문자를 구분하지 않으므로(같은 행을 가리킨다) 키도 소문자로 맞춘다.
// 쓰기 경로가 DB 행의 member_id 로 무효화해도 같은 키를 지우게 하기 위함이다.
func MemoTargetsCacheKey(viewerID string) string {
	return memoTargetsCacheKeyPrefix + strings.ToLower(viewerID)
}

// loadMemoGateTargets 는 게이트용 상대 ID 목록을 읽는다.
//
// LoadMemoTargets 와 같은 조회(같은 상한·truncated 판정)이되 memo 내용 조건을 두지 않는다.
// author_memo 조회는 Go 에서 빈 문자열만 거르므로, 게이트는 그보다 넓은(같거나 큰) 집합이어야
// 교집합 조회 결과가 전체 조회 결과와 같다. 내용 없는 행은 교집합 조회에서 그대로 걸러진다.
func loadMemoGateTargets(db *gorm.DB, viewerID string) ([]string, bool, error) {
	var targets []string
	if err := db.Table("g5_member_memo").
		Where("member_id = ?", viewerID).
		Limit(MemoTargetsLimit+1).
		Pluck("target_member_id", &targets).Error; err != nil {
		return nil, false, err
	}
	if len(targets) > MemoTargetsLimit {
		return nil, true, nil
	}
	if targets == nil {
		targets = []string{}
	}
	return targets, false, nil
}

// TargetSet 은 보는 사람이 메모를 남긴 상대 ID 집합을 돌려준다.
//
// ok=false 는 「모름」이다 — 호출자는 게이트 없이 기존 전체 조회를 해야 한다.
// (캐시 없음·Redis 오류/시간 초과·역직렬화 실패·DB 조회 실패·상한 초과)
// 캐시가 없으면 DB 에서 채운 뒤 저장한다. 저장 실패는 로그만 남기고 읽은 집합을 그대로 쓴다.
func (c *MemoTargetsCache) TargetSet(ctx context.Context, db *gorm.DB, viewerID string) (map[string]struct{}, bool) {
	if c == nil || c.store == nil || db == nil || viewerID == "" {
		return nil, false
	}
	key := MemoTargetsCacheKey(viewerID)

	rctx, cancel := context.WithTimeout(ctx, memoTargetsCacheReadTimeout)
	raw, err := c.store.Get(rctx, key).Bytes()
	cancel()
	switch {
	case err == nil:
		var ids []string
		if jerr := json.Unmarshal(raw, &ids); jerr != nil || ids == nil {
			logMemoTargetsCache("역직렬화 실패 — 전체 조회: %v", jerr)
			return nil, false
		}
		return memoTargetIDSet(ids), true
	case errors.Is(err, redis.Nil):
		// 캐시 없음 — 아래에서 DB 로 채운다.
	default:
		logMemoTargetsCache("읽기 실패 — 전체 조회: %v", err)
		return nil, false
	}

	ids, truncated, err := loadMemoGateTargets(db, viewerID)
	if err != nil {
		logMemoTargetsCache("대상 조회 실패 — 전체 조회: %v", err)
		return nil, false
	}
	if truncated {
		return nil, false
	}
	payload, err := json.Marshal(ids)
	if err == nil {
		wctx, wcancel := context.WithTimeout(context.WithoutCancel(ctx), memoTargetsCacheWriteTimeout)
		err = c.store.Set(wctx, key, payload, MemoTargetsCacheTTL).Err()
		wcancel()
	}
	if err != nil {
		logMemoTargetsCache("저장 실패(이번 요청은 읽은 집합 사용): %v", err)
	}
	return memoTargetIDSet(ids), true
}

// Invalidate 는 보는 사람의 메모 대상 집합 캐시를 지운다. 메모 쓰기 성공 뒤 호출한다.
// 실패는 로그만 남긴다(TTL 이 상한).
func (c *MemoTargetsCache) Invalidate(ctx context.Context, viewerID string) {
	if c == nil || c.store == nil || viewerID == "" {
		return
	}
	dctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), memoTargetsCacheDelTimeout)
	defer cancel()
	if err := c.store.Del(dctx, MemoTargetsCacheKey(viewerID)).Err(); err != nil {
		log.Printf("[memo-targets-cache] 무효화 실패: %v", err)
	}
}

func memoTargetIDSet(ids []string) map[string]struct{} {
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set
}
