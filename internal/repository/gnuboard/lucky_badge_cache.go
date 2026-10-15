package gnuboard

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// 행운 배지 조회를 줄이는 세 장치다.
//
//  1. 경험치 게이트(luckyLedgerGate): 지급 원장(g5_da_lucky_grant)의 게시판별 source_id 집합을 프로세스에
//     luckyLedgerTTL 동안 들고, 요청 id 와 교집합이 있을 때만 g5_na_xp 를 그 교집합으로 조회한다.
//     행운 경험치는 원장 INSERT 와 같은 트랜잭션에서만 쓰이므로 원장에 없는 글에는 행운 경험치 행이 없다.
//  2. 글 상세 캐시(LuckyBadgesForPostCached): 글 하나의 배지 원자료를 Redis 에 luckyBadgeCacheTTL 동안 둔다
//     (기록 없음도 캐시). 지급 성공 뒤 InvalidateLuckyBadge 로 지운다.
//
//  3. 포인트 게이트(luckyPointGate): 원장 밖 행운 포인트 행(옛 지급분)의 게시판별 최대 글번호(워터마크)를
//     프로세스에 luckyWatermarkTTL 동안 들고(요청 밖 백그라운드에서 게시판당 하나씩 읽는다 — 모르는 동안은 전체 조회), 요청 id 중 「워터마크 이하 또는 원장에 있는 id」로만 g5_point 를
//     조회한다. 새 지급은 전부 원장과 한 트랜잭션으로 기록되고 옛 지급은 더 늘지 않으므로, 둘 다 아닌 글에는
//     행운 포인트 행이 없다.
//
// 모두 실패하면 예전과 같은 전체 조회로 돌아간다(캐시는 성능용이지 정답의 근거가 아니다).

// luckyLedgerTTL 은 게시판별 원장 id 집합의 프로세스 캐시 수명이다.
const luckyLedgerTTL = 60 * time.Second

// luckyWatermarkTTL 은 게시판별 워터마크의 프로세스 캐시 수명이다. 원장 밖 행운 포인트 행은 더 생기지 않으므로 길게 둔다.
// 만료돼도 새 값을 읽을 때까지는 이전 값을 그대로 쓴다(뒤에서 다시 읽는다).
const luckyWatermarkTTL = 24 * time.Hour

// luckyWatermarkLoadTimeout 은 워터마크 백그라운드 조회 한 번의 시간 상한이다.
const luckyWatermarkLoadTimeout = 15 * time.Second

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

// luckyWatermark 는 원장 밖 행운 포인트 행의 게시판별 최대 글번호다.
type luckyWatermark struct {
	maxID uint64 // has 일 때만 뜻이 있다
	has   bool   // false 면 원장 밖 행운 포인트 행이 없다(원장 id 만 조회)
}

// luckyWatermarkLoader 는 한 게시판의 워터마크를 읽는 함수다(운영은 loadLuckyWatermark, 테스트는 바꿔 낄 수 있다).
type luckyWatermarkLoader func(ctx context.Context, db *gorm.DB, slug string) (luckyWatermark, error)

// luckyWatermarkLoad 는 백그라운드 조회에 쓰는 로더다. 테스트만 바꾼다(조회가 돌고 있지 않을 때).
var luckyWatermarkLoad luckyWatermarkLoader = loadLuckyWatermark

// luckyWatermarkRetryAfter 는 조회가 실패한 뒤 같은 게시판을 다시 조회하기까지의 최소 간격이다
// (무거운 조회가 실패를 되풀이하며 연달아 돌지 않게). 실패한 값은 캐시하지 않는다 — 그동안은 「모름」이다.
var luckyWatermarkRetryAfter = time.Minute

// luckyWatermarkEntry 는 한 게시판의 워터마크 상태다.
type luckyWatermarkEntry struct {
	wm       luckyWatermark
	loaded   bool      // 한 번이라도 조회에 성공했다(wm 이 뜻이 있다)
	expires  time.Time // loaded 일 때 다시 읽을 시각(지나도 새 값이 올 때까지 wm 을 쓴다)
	loading  bool      // 이 게시판 조회가 돌고 있다(게시판당 하나만)
	failedAt time.Time // 마지막 조회 실패 시각
}

// luckyWatermarkCache 는 게시판별 워터마크의 프로세스 캐시다. 요청 경로에서는 조회하지 않는다 —
// 없거나 만료되면 백그라운드 조회 하나를 띄우고(게시판당 동시에 하나), 그동안은 가진 값(없으면 「모름」)을 돌려준다.
type luckyWatermarkCache struct {
	mu      sync.Mutex
	entries map[string]luckyWatermarkEntry
	gen     uint64         // reset 마다 바뀐다 — 그 전에 떠난 조회 결과는 버린다
	wg      sync.WaitGroup // 돌고 있는 백그라운드 조회(테스트·reset 이 기다린다)
}

var luckyWatermarks = &luckyWatermarkCache{entries: make(map[string]luckyWatermarkEntry)}

// lookup 은 한 게시판의 워터마크를 돌려준다(ok=false 면 「모름」 — 호출부는 전체 조회). 절대 DB 를 기다리지 않는다.
// 값이 없거나 만료됐고 조회가 돌고 있지 않으면(실패 직후 luckyWatermarkRetryAfter 동안은 빼고) 백그라운드 조회를 띄운다.
func (c *luckyWatermarkCache) lookup(db *gorm.DB, slug string, now time.Time) (luckyWatermark, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, exists := c.entries[slug]
	if e.loaded && now.Before(e.expires) {
		return e.wm, true
	}
	if !e.loading && (e.failedAt.IsZero() || now.Sub(e.failedAt) >= luckyWatermarkRetryAfter) {
		if !exists && len(c.entries) >= luckyLedgerMaxBoards {
			c.entries = make(map[string]luckyWatermarkEntry)
		}
		e.loading = true
		c.entries[slug] = e
		c.wg.Add(1)
		go c.load(db, slug, c.gen, luckyWatermarkLoad)
	}
	return e.wm, e.loaded
}

// load 는 백그라운드에서 워터마크를 읽어 캐시에 넣는다. 실패(패닉 포함)는 로그만 남기고 이전 값을 그대로 둔다.
func (c *luckyWatermarkCache) load(db *gorm.DB, slug string, gen uint64, fn luckyWatermarkLoader) {
	defer c.wg.Done()
	ctx, cancel := context.WithTimeout(context.Background(), luckyWatermarkLoadTimeout)
	defer cancel()
	wm, err := func() (wm luckyWatermark, err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("panic: %v", r)
			}
		}()
		return fn(ctx, db, slug)
	}()

	now := time.Now()
	c.mu.Lock()
	if gen == c.gen {
		e := c.entries[slug]
		e.loading = false
		if err != nil {
			e.failedAt = now
		} else {
			e = luckyWatermarkEntry{wm: wm, loaded: true, expires: now.Add(luckyWatermarkTTL)}
		}
		c.entries[slug] = e
	}
	c.mu.Unlock()
	if err != nil {
		luckyLogRateLimited("[lucky] 포인트 워터마크 조회 실패 board=%s — 그동안 포인트는 전체 조회: %v", slug, err)
	}
}

// wait 는 돌고 있는 백그라운드 조회가 끝날 때까지 기다린다(테스트용).
func (c *luckyWatermarkCache) wait() {
	c.wg.Wait()
}

// reset 은 돌고 있는 조회를 기다린 뒤 캐시를 비운다(테스트용).
func (c *luckyWatermarkCache) reset() {
	c.wg.Wait()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]luckyWatermarkEntry)
	c.gen++
}

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

// luckyWatermarkSQL 은 한 게시판에서 원장에 없는 행운 포인트 행의 최대 rel_id(숫자)를 구한다.
// g5_point 는 idx_lucky_display(po_rel_table, po_rel_id, po_rel_action) 의 po_rel_table 범위만 인덱스로 읽고
// (세 컬럼이 모두 인덱스에 있어 행을 읽지 않는다), 원장은 uq_lucky_source(source_table, source_id) 앞 두 컬럼으로
// 존재만 확인한다. 숫자가 아닌 rel_id 는 CAST 가 앞자리 숫자(없으면 0)로 바꾼다 — 그런 행은 요청 id(10진 문자열)와
// 문자열로 같아질 수 없으므로 포인트 조회 결과에 원래 나오지 않는다.
const luckyWatermarkSQL = `SELECT MAX(CAST(p.po_rel_id AS UNSIGNED)) AS wm
FROM g5_point p
WHERE p.po_rel_table = ? AND p.po_rel_action = ?
  AND NOT EXISTS (SELECT 1 FROM g5_da_lucky_grant g WHERE g.source_table = ? AND g.source_id = p.po_rel_id)`

// loadLuckyWatermark 는 한 게시판의 워터마크를 1쿼리로 읽는다(백그라운드 전용 — 큰 게시판에서는 수 초 걸릴 수 있다).
func loadLuckyWatermark(ctx context.Context, db *gorm.DB, slug string) (luckyWatermark, error) {
	row := db.WithContext(ctx).Raw(luckyWatermarkSQL, slug, luckyRelAction, slug).Row()
	if row == nil {
		return luckyWatermark{}, errors.New("워터마크 조회 실패: 결과 행 없음")
	}
	var raw sql.NullString // 집계라 항상 한 행이다. 원장 밖 행이 없으면 NULL
	if err := row.Scan(&raw); err != nil {
		return luckyWatermark{}, err
	}
	if !raw.Valid {
		return luckyWatermark{}, nil
	}
	v, err := strconv.ParseUint(strings.TrimSpace(raw.String), 10, 64)
	if err != nil {
		return luckyWatermark{}, fmt.Errorf("워터마크 해석 실패 %q: %w", raw.String, err)
	}
	return luckyWatermark{maxID: v, has: true}, nil
}

// luckyPointGate 는 요청 id 중 행운 포인트 행이 있을 수 있는 id 만 돌려준다(순서 유지) — 워터마크 이하이거나
// 원장(ledger)에 있는 id. 원장 조회가 실패했거나(ledgerErr) 워터마크를 아직 모르면(백그라운드 조회 전·실패)
// 요청 id 를 그대로 돌려준다(fail-open). 숫자로 읽을 수 없는 id 도 남긴다(fail-open).
func luckyPointGate(db *gorm.DB, slug string, ids []string, ledger map[string]struct{}, ledgerErr error) []string {
	if ledgerErr != nil {
		return ids
	}
	wm, ok := luckyWatermarks.lookup(db, slug, time.Now())
	if !ok {
		return ids
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, ok := ledger[id]; ok {
			out = append(out, id)
			continue
		}
		n, perr := strconv.ParseUint(id, 10, 64)
		if perr != nil || (wm.has && n <= wm.maxID) {
			out = append(out, id)
		}
	}
	return out
}

// luckyLogRateLimited 는 luckyLedgerLogInterval 에 한 번만 로그를 남긴다(요청마다 같은 실패가 찍히지 않게).
func luckyLogRateLimited(format string, args ...any) {
	now := time.Now().UnixNano()
	last := luckyLedgerLastLog.Load()
	if now-last >= int64(luckyLedgerLogInterval) && luckyLedgerLastLog.CompareAndSwap(last, now) {
		log.Printf(format, args...)
	}
}

// luckyLedgerGate 는 요청 id 중 지급 원장(set)에 있는 id 만 돌려준다(순서 유지). 원장 조회가 실패했으면(err)
// 요청 id 를 그대로 돌려준다(fail-open — 예전처럼 전부 조회).
func luckyLedgerGate(ids []string, set map[string]struct{}, err error) []string {
	if err != nil {
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

// SetLuckyBadgeStoreForTest 는 저장소를 바꿔 끼우고 원래대로 돌리는 함수를 돌려준다. 원장 id·워터마크 프로세스 캐시도 비운다.
// 테스트 전용이다 — 운영 코드는 SetLuckyBadgeRedis 를 쓴다.
func SetLuckyBadgeStoreForTest(s LuckyBadgeStore) (restore func()) {
	luckyBadgeStoreMu.Lock()
	prev := luckyBadgeStore
	luckyBadgeStore = s
	luckyBadgeStoreMu.Unlock()
	luckyLedger.reset()
	luckyWatermarks.reset()
	return func() {
		setLuckyBadgeStore(prev)
		luckyLedger.reset()
		luckyWatermarks.reset()
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
	luckyLedger.forget(slug) // 워터마크는 원장 밖(옛 지급) 행만 보므로 새 지급으로 바뀌지 않는다 — 지우지 않는다

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
