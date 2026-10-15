package gnuboard

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// ⛔ 이 파일이 지키는 계약 (행운 배지 조회 줄이기):
//
//   - 경험치 게이트: 요청 id 와 지급 원장 교집합이 없으면 g5_na_xp 를 조회하지 않고, 있으면 교집합 id 로만 조회한다.
//     원장 조회가 실패하면 예전처럼 요청 id 전부로 조회한다. 원장 id 집합은 게시판별로 프로세스에 캐시된다.
//   - 글 상세 캐시: 적중이면 DB 쿼리 0, 미스면 조회 후 저장(기록 없음도), 저장소 오류면 DB 조회로 같은 값을 낸다.
//     캐시에서 꺼낸 값은 DB 에서 바로 읽은 값과 같다.
//   - 지급 뒤 InvalidateLuckyBadge 는 글 상세 키를 지우고 그 게시판 원장 캐시를 비운다.

// fakeLuckyStore 는 메모리 LuckyBadgeStore 다. getErr·setErr 로 오류를 흉내 낸다.
type fakeLuckyStore struct {
	mu     sync.Mutex
	data   map[string][]byte
	ttls   map[string]time.Duration
	dels   []string
	getErr error
	setErr error
}

func newFakeLuckyStore() *fakeLuckyStore {
	return &fakeLuckyStore{data: map[string][]byte{}, ttls: map[string]time.Duration{}}
}

func (f *fakeLuckyStore) Get(_ context.Context, key string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getErr != nil {
		return nil, f.getErr
	}
	v, ok := f.data[key]
	if !ok {
		return nil, errors.New("miss")
	}
	return v, nil
}

func (f *fakeLuckyStore) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.setErr != nil {
		return f.setErr
	}
	f.data[key] = value
	f.ttls[key] = ttl
	return nil
}

func (f *fakeLuckyStore) Del(_ context.Context, keys ...string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, k := range keys {
		delete(f.data, k)
		f.dels = append(f.dels, k)
	}
	return nil
}

func (f *fakeLuckyStore) has(key string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.data[key]
	return ok
}

func countSQL(sqls []string, table string) int {
	n := 0
	for _, s := range sqls {
		if strings.Contains(s, table) {
			n++
		}
	}
	return n
}

// TestLuckyLedgerGate_NoIntersectionSkipsExp — 원장에 없는 글뿐이면 g5_na_xp 조회가 없다.
func TestLuckyLedgerGate_NoIntersectionSkipsExp(t *testing.T) {
	db, counter := newLuckyBadgeTestDB(t, true)
	got, err := LuckyBadgesByWrID(db, "free", []int{101, 105}, LuckyTierNames{})
	if err != nil {
		t.Fatalf("조회 실패: %v", err)
	}
	sqls := counter.snapshot()
	if n := countSQL(sqls, "g5_na_xp"); n != 0 {
		t.Errorf("경험치 조회가 없어야 한다: %v", sqls)
	}
	if n := countSQL(sqls, "g5_da_lucky_grant"); n != 1 {
		t.Errorf("원장은 1번 읽어야 한다: %v", sqls)
	}
	if got[101] != (LuckyBadge{Points: 73}) {
		t.Errorf("포인트 배지는 그대로: %+v", got[101])
	}

	// 원장 집합은 캐시된다 — 다음 조회는 원장을 다시 읽지 않는다.
	counter.reset()
	if _, err := LuckyBadgesByWrID(db, "free", []int{101}, LuckyTierNames{}); err != nil {
		t.Fatalf("조회 실패: %v", err)
	}
	if sqls := counter.snapshot(); len(sqls) != 1 || countSQL(sqls, "g5_point") != 1 {
		t.Errorf("원장 캐시 적중이면 g5_point 1쿼리뿐이어야 한다: %v", sqls)
	}
}

// TestLuckyLedgerGate_IntersectionOnly — 경험치는 원장 교집합 id 로만 조회하고 값은 예전과 같다.
func TestLuckyLedgerGate_IntersectionOnly(t *testing.T) {
	db, counter := newLuckyBadgeTestDB(t, true)
	got, err := LuckyBadgesByWrID(db, "free", []int{101, 102, 105}, LuckyTierNames{})
	if err != nil {
		t.Fatalf("조회 실패: %v", err)
	}
	var xp string
	for _, s := range counter.snapshot() {
		if strings.Contains(s, "g5_na_xp") {
			xp = s
		}
	}
	if xp == "" {
		t.Fatal("교집합(102)이 있으니 경험치를 조회해야 한다")
	}
	if !strings.Contains(xp, "102") || strings.Contains(xp, "101") || strings.Contains(xp, "105") {
		t.Errorf("경험치 조회는 교집합 id(102)만이어야 한다: %s", xp)
	}
	want := LuckyBadge{Exp: 37, Tier: "앙팡타임", At: "2026-10-01T14:23:05+09:00"}
	if got[102] != want {
		t.Errorf("wr 102: got %+v want %+v", got[102], want)
	}
}

// TestLuckyLedgerGate_LedgerErrorFailsOpen — 원장 조회가 실패하면 요청 id 전부로 경험치를 조회한다(값 그대로).
func TestLuckyLedgerGate_LedgerErrorFailsOpen(t *testing.T) {
	db, counter := newLuckyBadgeTestDB(t, true)
	if err := db.Exec(`DROP TABLE g5_da_lucky_grant`).Error; err != nil {
		t.Fatalf("원장 삭제 실패: %v", err)
	}
	counter.reset()
	got, err := LuckyBadgesByWrID(db, "free", []int{101, 102, 103}, LuckyTierNames{})
	if err != nil {
		t.Fatalf("원장 실패는 조회 에러가 아니다: %v", err)
	}
	var xp string
	for _, s := range counter.snapshot() {
		if strings.Contains(s, "g5_na_xp") {
			xp = s
		}
	}
	for _, id := range []string{"101", "102", "103"} {
		if !strings.Contains(xp, id) {
			t.Errorf("원장 실패면 요청 id 전부(%s 포함)로 조회해야 한다: %s", id, xp)
		}
	}
	if got[102].Exp != 37 || got[103].Exp != 41 || got[103].Points != 58 {
		t.Errorf("값은 예전과 같아야 한다: %+v", got)
	}
	// 실패는 캐시하지 않는다 — 다음 조회도 원장을 다시 시도한다.
	counter.reset()
	_, _ = LuckyBadgesByWrID(db, "free", []int{101}, LuckyTierNames{})
	if n := countSQL(counter.snapshot(), "g5_da_lucky_grant"); n != 1 {
		t.Errorf("원장 실패는 캐시하지 않아야 한다, 원장 조회 %d회", n)
	}
}

// TestLuckyBadgesForPostCached_MissStoresThenHit — 미스면 DB 조회 후 저장(TTL 10분), 적중이면 쿼리 0 이고 값이 같다.
func TestLuckyBadgesForPostCached_MissStoresThenHit(t *testing.T) {
	db, counter := newLuckyBadgeTestDB(t, true)
	store := newFakeLuckyStore()
	restore := SetLuckyBadgeStoreForTest(store)
	defer restore()

	names := LuckyTierNames{}
	for _, id := range []int{101, 102, 103, 105, 106} { // 105 = 기록 없음
		direct, err := LuckyBadgesByWrID(db, "free", []int{id}, names)
		if err != nil {
			t.Fatalf("직접 조회 실패: %v", err)
		}
		key := LuckyBadgeCacheKey("free", id)

		counter.reset()
		miss, err := LuckyBadgesForPostCached(context.Background(), db, "free", id, names)
		if err != nil {
			t.Fatalf("미스 조회 실패: %v", err)
		}
		if countSQL(counter.snapshot(), "g5_point") != 1 {
			t.Errorf("wr %d 미스면 DB 를 읽어야 한다: %v", id, counter.snapshot())
		}
		if !store.has(key) || store.ttls[key] != LuckyBadgeCacheTTL {
			t.Errorf("wr %d 미스 뒤 TTL %v 로 저장해야 한다(기록 없음도): has=%v ttl=%v", id, LuckyBadgeCacheTTL, store.has(key), store.ttls[key])
		}

		counter.reset()
		hit, err := LuckyBadgesForPostCached(context.Background(), db, "free", id, names)
		if err != nil {
			t.Fatalf("적중 조회 실패: %v", err)
		}
		if n := len(counter.snapshot()); n != 0 {
			t.Errorf("wr %d 적중이면 쿼리 0 이어야 한다, got %d", id, n)
		}
		for _, m := range []map[int]LuckyBadge{miss, hit} {
			if len(m) != len(direct) || m[id] != direct[id] {
				t.Errorf("wr %d 캐시 값이 직접 조회와 달라: got %+v want %+v", id, m, direct)
			}
		}
	}
	if got := LuckyBadgeCacheKey("free", 103); got != "lucky:badge:free:103" {
		t.Errorf("키 형식: %s", got)
	}
}

// TestLuckyBadgesForPostCached_TierResolvedOnRead — 캐시는 원자료라 단계 이름은 읽을 때 설정으로 해석된다.
func TestLuckyBadgesForPostCached_TierResolvedOnRead(t *testing.T) {
	db, _ := newLuckyBadgeTestDB(t, true)
	if err := db.Exec(`INSERT INTO g5_point (mb_id, po_datetime, po_content, po_point, po_rel_table, po_rel_id, po_rel_action)
		VALUES ('m7', '2026-10-01 03:10:00', '테스트구간 럭키 포인트', 21, 'free', '107', '@lucky')`).Error; err != nil {
		t.Fatalf("시드 실패: %v", err)
	}
	restore := SetLuckyBadgeStoreForTest(newFakeLuckyStore())
	defer restore()

	first, _ := LuckyBadgesForPostCached(context.Background(), db, "free", 107, LuckyTierNames{})
	if first[107].Tier != "" {
		t.Fatalf("설정 이름이 없으면 단계 없음: %+v", first[107])
	}
	hit, _ := LuckyBadgesForPostCached(context.Background(), db, "free", 107, LuckyTierNames{Names: []string{"테스트구간"}})
	if hit[107].Tier != "테스트구간" || hit[107].At != "2026-10-01T03:10:00+09:00" || hit[107].Points != 21 {
		t.Errorf("캐시 적중에서도 현재 설정으로 단계를 해석해야 한다: %+v", hit[107])
	}
}

// TestLuckyBadgesForPostCached_StoreErrorFallsBack — 저장소 오류·저장소 없음이면 DB 를 읽어 같은 값을 낸다.
func TestLuckyBadgesForPostCached_StoreErrorFallsBack(t *testing.T) {
	db, counter := newLuckyBadgeTestDB(t, true)
	want := LuckyBadge{Points: 58, Exp: 41, Tier: "앙팡팡타임", At: "2026-10-01T10:00:00+09:00"}

	store := newFakeLuckyStore()
	store.getErr = errors.New("redis down")
	store.setErr = errors.New("redis down")
	restore := SetLuckyBadgeStoreForTest(store)
	for i := 0; i < 2; i++ {
		counter.reset()
		got, err := LuckyBadgesForPostCached(context.Background(), db, "free", 103, LuckyTierNames{})
		if err != nil {
			t.Fatalf("저장소 오류는 조회 에러가 아니다: %v", err)
		}
		if got[103] != want {
			t.Errorf("저장소 오류: got %+v want %+v", got[103], want)
		}
		if countSQL(counter.snapshot(), "g5_point") != 1 {
			t.Errorf("저장소 오류면 매번 DB 를 읽어야 한다: %v", counter.snapshot())
		}
	}
	restore()

	restore = SetLuckyBadgeStoreForTest(nil)
	defer restore()
	got, err := LuckyBadgesForPostCached(context.Background(), db, "free", 103, LuckyTierNames{})
	if err != nil || got[103] != want {
		t.Errorf("저장소 없음: got %+v err %v", got[103], err)
	}
}

// TestLuckyBadgesForPostCached_PartialFailureNotCached — 경험치 조회가 실패하면 포인트는 돌려주되 캐시에 쓰지 않는다.
func TestLuckyBadgesForPostCached_PartialFailureNotCached(t *testing.T) {
	db, _ := newLuckyBadgeTestDB(t, false) // g5_na_xp 없음
	store := newFakeLuckyStore()
	restore := SetLuckyBadgeStoreForTest(store)
	defer restore()

	got, err := LuckyBadgesForPostCached(context.Background(), db, "free", 103, LuckyTierNames{})
	if err == nil {
		t.Fatal("경험치 조회 실패는 에러로 알려야 한다")
	}
	if got[103].Points != 58 {
		t.Errorf("포인트는 남아야 한다: %+v", got)
	}
	if store.has(LuckyBadgeCacheKey("free", 103)) {
		t.Error("부분 실패 결과는 캐시에 쓰면 안 된다")
	}
}

// TestInvalidateLuckyBadge — 글 상세 키를 지우고 그 게시판 원장 캐시를 비운다.
func TestInvalidateLuckyBadge(t *testing.T) {
	db, counter := newLuckyBadgeTestDB(t, true)
	store := newFakeLuckyStore()
	restore := SetLuckyBadgeStoreForTest(store)
	defer restore()

	if _, err := LuckyBadgesForPostCached(context.Background(), db, "free", 105, LuckyTierNames{}); err != nil {
		t.Fatalf("조회 실패: %v", err)
	}
	if _, err := LuckyBadgesByWrID(db, "free", []int{105}, LuckyTierNames{}); err != nil { // 원장 캐시 적재
		t.Fatalf("조회 실패: %v", err)
	}
	key := LuckyBadgeCacheKey("free", 105)
	if !store.has(key) {
		t.Fatal("미스 뒤 저장되어 있어야 한다")
	}

	InvalidateLuckyBadge("free", "105")
	if store.has(key) {
		t.Error("지급 뒤 글 상세 키가 지워져야 한다")
	}
	counter.reset()
	if _, err := LuckyBadgesByWrID(db, "free", []int{105}, LuckyTierNames{}); err != nil {
		t.Fatalf("조회 실패: %v", err)
	}
	if n := countSQL(counter.snapshot(), "g5_da_lucky_grant"); n != 1 {
		t.Errorf("지급 뒤 그 게시판 원장 캐시를 다시 읽어야 한다, 원장 조회 %d회", n)
	}

	// 잘못된 id 는 키를 지우지 않는다(원장 캐시만 비운다).
	store.dels = nil
	InvalidateLuckyBadge("free", "abc")
	if len(store.dels) != 0 {
		t.Errorf("숫자가 아닌 id 는 DEL 하지 않는다: %v", store.dels)
	}
}
