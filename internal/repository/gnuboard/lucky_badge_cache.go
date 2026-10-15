package gnuboard

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// 행운 배지 조회를 줄이는 두 장치다.
//
//  1. 경험치 게이트(luckyLedgerGate): 지급 원장(g5_da_lucky_grant)의 게시판별 source_id 집합을 프로세스에
//     luckyLedgerTTL 동안 들고, 요청 id 와 교집합이 있을 때만 g5_na_xp 를 그 교집합으로 조회한다.
//     행운 경험치는 원장 INSERT 와 같은 트랜잭션에서만 쓰이므로 원장에 없는 글에는 행운 경험치 행이 없다.
//  2. 글 상세 캐시(LuckyBadgesForPostCached): 글 하나의 배지 원자료를 Redis 에 luckyBadgeCacheTTL 동안 둔다
//     (기록 없음도 캐시). 지급 성공 뒤 InvalidateLuckyBadge 로 지운다.
//
// 둘 다 실패하면 예전과 같은 전체 조회로 돌아간다(캐시는 성능용이지 정답의 근거가 아니다).

// luckyLedgerTTL 은 게시판별 원장 id 집합의 프로세스 캐시 수명이다.
const luckyLedgerTTL = 60 * time.Second

// luckyLedgerMaxBoards 는 프로세스 캐시에 담는 게시판 수 상한이다. 넘으면 통째로 비운다(게시판 수는 이보다 훨씬 적다).
const luckyLedgerMaxBoards = 1000

// luckyLedgerLogInterval 은 원장 조회·배지 캐시 저장 실패 로그의 최소 간격이다(요청마다 찍히지 않게).
const luckyLedgerLogInterval = time.Minute

// LuckyBadgeCacheTTL 은 글 상세 배지 캐시(Redis)의 수명이다.
const LuckyBadgeCacheTTL = 10 * time.Minute

// luckyBadgeCacheKeyPrefix 는 글 상세 배지 캐시 키 접두사다 — `lucky:badge:{slug}:{wr_id}`. 백엔드만 읽고 쓴다.
const luckyBadgeCacheKeyPrefix = "lucky:badge:"

// 글 상세 캐시 Redis 호출 시간 상한. 읽기는 요청 경로라 짧게, 넘기면 DB 조회로 간다.
const (
	luckyBadgeCacheReadTimeout  = 100 * time.Millisecond
	luckyBadgeCacheWriteTimeout = 200 * time.Millisecond
	luckyBadgeCacheDelTimeout   = 500 * time.Millisecond
)

// luckyLedgerEntry 는 한 게시판의 원장 source_id 집합과 만료 시각이다.
type luckyLedgerEntry struct {
	ids     map[string]struct{}
	expires time.Time
}

// luckyLedgerCache 는 게시판별 원장 id 집합의 프로세스 캐시다.
type luckyLedgerCache struct {
	mu      sync.Mutex
	entries map[string]luckyLedgerEntry
}

var luckyLedger = &luckyLedgerCache{entries: make(map[string]luckyLedgerEntry)}

// luckyLedgerLastLog 는 마지막 실패 로그 시각(UnixNano)이다.
var luckyLedgerLastLog atomic.Int64

func (c *luckyLedgerCache) get(slug string, now time.Time) (map[string]struct{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[slug]
	if !ok || !now.Before(e.expires) {
		return nil, false
	}
	return e.ids, true
}

func (c *luckyLedgerCache) put(slug string, ids map[string]struct{}, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= luckyLedgerMaxBoards {
		c.entries = make(map[string]luckyLedgerEntry)
	}
	c.entries[slug] = luckyLedgerEntry{ids: ids, expires: now.Add(luckyLedgerTTL)}
}

func (c *luckyLedgerCache) forget(slug string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, slug)
}

func (c *luckyLedgerCache) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]luckyLedgerEntry)
}

// luckyLedgerIDs 는 한 게시판의 원장 source_id 집합을 돌려준다(프로세스 캐시 → 없으면 원장 1쿼리).
// 조회 실패는 캐시하지 않는다.
func luckyLedgerIDs(db *gorm.DB, slug string) (map[string]struct{}, error) {
	now := time.Now()
	if ids, ok := luckyLedger.get(slug, now); ok {
		return ids, nil
	}
	// uq_lucky_source(source_table, source_id, kind) 의 앞 두 컬럼만으로 답한다(인덱스만 읽음).
	var rows []string
	if err := db.Raw("SELECT DISTINCT source_id FROM g5_da_lucky_grant WHERE source_table = ?", slug).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	ids := make(map[string]struct{}, len(rows))
	for _, id := range rows {
		ids[id] = struct{}{}
	}
	luckyLedger.put(slug, ids, now)
	return ids, nil
}

// luckyLogRateLimited 는 luckyLedgerLogInterval 에 한 번만 로그를 남긴다(요청마다 같은 실패가 찍히지 않게).
func luckyLogRateLimited(format string, args ...any) {
	now := time.Now().UnixNano()
	last := luckyLedgerLastLog.Load()
	if now-last >= int64(luckyLedgerLogInterval) && luckyLedgerLastLog.CompareAndSwap(last, now) {
		log.Printf(format, args...)
	}
}

// luckyLedgerGate 는 요청 id 중 지급 원장에 있는 id 만 돌려준다(순서 유지). 원장 조회가 실패하면 요청 id 를
// 그대로 돌려준다(fail-open — 예전처럼 전부 조회).
func luckyLedgerGate(db *gorm.DB, slug string, ids []string) []string {
	set, err := luckyLedgerIDs(db, slug)
	if err != nil {
		luckyLogRateLimited("[lucky] 지급 원장 조회 실패 board=%s — 경험치는 전체 조회: %v", slug, err)
		return ids
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, ok := set[id]; ok {
			out = append(out, id)
		}
	}
	return out
}

// LuckyBadgeStore 는 글 상세 배지 캐시가 쓰는 저장소다(운영은 Redis). Get 은 없는 키에 에러를 돌려준다.
type LuckyBadgeStore interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Del(ctx context.Context, keys ...string) error
}

// redisLuckyBadgeStore 는 go-redis 클라이언트로 LuckyBadgeStore 를 구현한다.
type redisLuckyBadgeStore struct {
	c *redis.Client
}

// Get 은 키 값을 읽는다(없으면 redis.Nil).
func (s redisLuckyBadgeStore) Get(ctx context.Context, key string) ([]byte, error) {
	return s.c.Get(ctx, key).Bytes()
}

// Set 은 키 값을 TTL 과 함께 쓴다.
func (s redisLuckyBadgeStore) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return s.c.Set(ctx, key, value, ttl).Err()
}

// Del 은 키들을 지운다.
func (s redisLuckyBadgeStore) Del(ctx context.Context, keys ...string) error {
	return s.c.Del(ctx, keys...).Err()
}

var (
	luckyBadgeStoreMu sync.RWMutex
	luckyBadgeStore   LuckyBadgeStore
)

// SetLuckyBadgeRedis 는 글 상세 배지 캐시에 쓸 Redis 클라이언트를 주입한다. nil 이면 캐시 없이 매번 DB 를 읽는다.
// 라우팅 개시 전에 부를 것.
func SetLuckyBadgeRedis(c *redis.Client) {
	if c == nil {
		setLuckyBadgeStore(nil)
		return
	}
	setLuckyBadgeStore(redisLuckyBadgeStore{c: c})
}

// SetLuckyBadgeStoreForTest 는 저장소를 바꿔 끼우고 원래대로 돌리는 함수를 돌려준다. 원장 id 프로세스 캐시도 비운다.
// 테스트 전용이다 — 운영 코드는 SetLuckyBadgeRedis 를 쓴다.
func SetLuckyBadgeStoreForTest(s LuckyBadgeStore) (restore func()) {
	luckyBadgeStoreMu.Lock()
	prev := luckyBadgeStore
	luckyBadgeStore = s
	luckyBadgeStoreMu.Unlock()
	luckyLedger.reset()
	return func() {
		setLuckyBadgeStore(prev)
		luckyLedger.reset()
	}
}

func setLuckyBadgeStore(s LuckyBadgeStore) {
	luckyBadgeStoreMu.Lock()
	luckyBadgeStore = s
	luckyBadgeStoreMu.Unlock()
}

func currentLuckyBadgeStore() LuckyBadgeStore {
	luckyBadgeStoreMu.RLock()
	defer luckyBadgeStoreMu.RUnlock()
	return luckyBadgeStore
}

// LuckyBadgeCacheKey 는 글 상세 배지 캐시 키 `lucky:badge:{slug}:{wr_id}` 다.
func LuckyBadgeCacheKey(slug string, wrID int) string {
	return luckyBadgeCacheKeyPrefix + slug + ":" + strconv.Itoa(wrID)
}

// LuckyBadgesForPostCached 는 글 상세용으로 글 하나(wrID)의 배지를 돌려준다. 돌려주는 맵·에러의 뜻은
// LuckyBadgesByWrID(db, slug, []int{wrID}, names) 와 같다.
//
// Redis 에 원자료(포인트·경험치 행)를 LuckyBadgeCacheTTL 동안 캐시한다 — 기록 없음도 빈 원자료로 캐시한다.
// 단계 이름은 읽을 때마다 names 로 해석하므로 설정이 바뀌어도 캐시 때문에 달라지지 않는다.
// 미스·Redis 오류·해석 실패면 예전처럼 DB 를 읽는다(이때는 경험치 게이트 없이 전체 조회 — 캐시에 담길 값이라
// 다른 프로세스의 원장 캐시가 늦어도 틀린 값을 10분 동안 들고 있지 않게). 조회가 부분 실패하면 캐시에 쓰지 않는다.
func LuckyBadgesForPostCached(ctx context.Context, db *gorm.DB, slug string, wrID int, names LuckyTierNames) (map[int]LuckyBadge, error) {
	if db == nil || slug == "" || wrID <= 0 {
		return make(map[int]LuckyBadge), nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	key := LuckyBadgeCacheKey(slug, wrID)
	store := currentLuckyBadgeStore()
	if store != nil {
		gctx, cancel := context.WithTimeout(ctx, luckyBadgeCacheReadTimeout)
		raw, err := store.Get(gctx, key)
		cancel()
		if err == nil {
			var rows luckyBadgeRows
			if jerr := json.Unmarshal(raw, &rows); jerr == nil {
				return mergeLuckyBadgeRows(rows, names), nil
			}
		}
	}

	rows, err := fetchLuckyBadgeRows(db, slug, []string{strconv.Itoa(wrID)}, false)
	if err != nil {
		return mergeLuckyBadgeRows(rows, names), err
	}
	if store != nil {
		if raw, jerr := json.Marshal(rows); jerr == nil {
			sctx, cancel := context.WithTimeout(context.Background(), luckyBadgeCacheWriteTimeout)
			serr := store.Set(sctx, key, raw, LuckyBadgeCacheTTL)
			cancel()
			if serr != nil { // 실패해도 다음 요청이 다시 읽는다
				luckyLogRateLimited("[lucky] 배지 캐시 저장 실패 board=%s wr_id=%d: %v", slug, wrID, serr)
			}
		}
	}
	return mergeLuckyBadgeRows(rows, names), nil
}

// InvalidateLuckyBadge 는 한 글·댓글(slug, wr_id 문자열)에 행운 지급이 새로 기록된 뒤 부른다.
// 이 프로세스의 그 게시판 원장 id 캐시를 비우고, 글 상세 배지 캐시 키를 지운다.
//
// 반드시 지급 트랜잭션이 커밋된 뒤 부른다(커밋 전에 지우면 그 사이 낡은 값이 다시 적재될 수 있다).
// 에러는 돌려주지 않는다 — 지우지 못하면 캐시 TTL 로 자연 만료된다.
func InvalidateLuckyBadge(slug, sourceID string) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return
	}
	luckyLedger.forget(slug)

	wrID, err := strconv.Atoi(strings.TrimSpace(sourceID))
	if err != nil || wrID <= 0 {
		return
	}
	store := currentLuckyBadgeStore()
	if store == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), luckyBadgeCacheDelTimeout)
	defer cancel()
	if err := store.Del(ctx, LuckyBadgeCacheKey(slug, wrID)); err != nil {
		log.Printf("[lucky] 배지 캐시 삭제 실패 board=%s wr_id=%d: %v", slug, wrID, err)
	}
}
