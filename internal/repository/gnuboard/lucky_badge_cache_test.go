package gnuboard

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
)

// ⛔ 이 파일이 지키는 계약 (행운 배지 조회 줄이기):
//
//   - 경험치 게이트: 요청 id 와 지급 원장 교집합이 없으면 g5_na_xp 를 조회하지 않고, 있으면 교집합 id 로만 조회한다.
//     원장 조회가 실패하면 예전처럼 요청 id 전부로 조회한다. 원장 id 집합은 게시판별로 프로세스에 캐시된다.
//   - 글 상세 캐시: 적중이면 DB 쿼리 0, 미스면 조회 후 저장(기록 없음도), 저장소 오류면 DB 조회로 같은 값을 낸다.
//     캐시에서 꺼낸 값은 DB 에서 바로 읽은 값과 같다.
//   - 지급 뒤 InvalidateLuckyBadge 는 글 상세 키를 지우고 그 게시판 원장 캐시를 비운다.
//   - 포인트 게이트: g5_point 는 「워터마크(원장 밖 행운 포인트의 최대 글번호) 이하 또는 원장」 id 로만 조회하고,
//     그런 id 가 없으면 조회하지 않는다. 원장 조회가 실패하거나 워터마크를 아직 모르면 요청 id 전부로 조회한다. 값은 예전과 같다.
//   - 워터마크는 요청 안에서 읽지 않는다: 없으면 「모름」을 바로 돌려주고 백그라운드 조회 하나만 띄운다(게시판당 동시에 하나).
//     실패는 캐시하지 않고 재시도 간격 뒤 다시 시도한다. 만료돼도 새 값이 올 때까지 이전 값을 쓴다.

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
	warmLuckyWatermarkForTest(t, db, "free")
	counter.reset()
	got, err := LuckyBadgesByWrID(db, "free", []int{101, 105}, LuckyTierNames{})
	if err != nil {
		t.Fatalf("조회 실패: %v", err)
	}
	sqls := counter.snapshot()
	if n := countSQL(sqls, "g5_na_xp"); n != 0 {
		t.Errorf("경험치 조회가 없어야 한다: %v", sqls)
	}
	if n := countSQL(sqls, "SELECT DISTINCT source_id"); n != 1 {
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
	if n := countSQL(counter.snapshot(), "SELECT DISTINCT source_id"); n != 1 {
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
	warmLuckyWatermarkForTest(t, db, "free")
	counter.reset()
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
	if n := countSQL(counter.snapshot(), "SELECT DISTINCT source_id"); n != 1 {
		t.Errorf("지급 뒤 그 게시판 원장 캐시를 다시 읽어야 한다, 원장 조회 %d회", n)
	}
	if n := countSQL(counter.snapshot(), "MAX(CAST"); n != 0 {
		t.Errorf("워터마크는 새 지급으로 바뀌지 않는다 — 다시 읽지 않는다, 워터마크 조회 %d회", n)
	}

	// 잘못된 id 는 키를 지우지 않는다(원장 캐시만 비운다).
	store.dels = nil
	InvalidateLuckyBadge("free", "abc")
	if len(store.dels) != 0 {
		t.Errorf("숫자가 아닌 id 는 DEL 하지 않는다: %v", store.dels)
	}
}

// luckyPointQueryOf 는 기록된 SQL 중 배지 포인트 조회(g5_point 본 조회)를 돌려준다(없으면 "").
// 워터마크 쿼리도 g5_point 를 읽으므로 선택 컬럼으로 구분한다.
func luckyPointQueryOf(sqls []string) string {
	for _, s := range sqls {
		if strings.Contains(s, "po_point AS amt") {
			return s
		}
	}
	return ""
}

// luckyPointQueryHas 는 포인트 조회 SQL 의 IN 목록에 id 가 있는지 본다(드라이버마다 문자열 따옴표가 달라 둘 다 본다).
func luckyPointQueryHas(q, id string) bool {
	return strings.Contains(q, "'"+id+"'") || strings.Contains(q, `"`+id+`"`)
}

// TestLuckyPointGate_WatermarkOrLedgerOnly — 워터마크(원장 밖 최대 글번호 101) 이하·원장 id 만 조회하고, 둘 다 아닌 id 는
// 빠진다. 원장에 있는 글(106)의 옛 문구 행은 워터마크를 올리지 않는다. 값은 예전과 같다.
func TestLuckyPointGate_WatermarkOrLedgerOnly(t *testing.T) {
	db, counter := newLuckyBadgeTestDB(t, true)
	warmLuckyWatermarkForTest(t, db, "free")
	counter.reset()
	got, err := LuckyBadgesByWrID(db, "free", []int{100, 101, 103, 105, 110}, LuckyTierNames{})
	if err != nil {
		t.Fatalf("조회 실패: %v", err)
	}
	q := luckyPointQueryOf(counter.snapshot())
	if q == "" {
		t.Fatal("워터마크 이하·원장 id 가 있으니 포인트를 조회해야 한다")
	}
	for _, id := range []string{"100", "101", "103"} {
		if !luckyPointQueryHas(q, id) {
			t.Errorf("포인트 조회에 %s 가 있어야 한다: %s", id, q)
		}
	}
	for _, id := range []string{"105", "110"} {
		if luckyPointQueryHas(q, id) {
			t.Errorf("워터마크 초과·원장 밖 %s 는 조회하지 않는다: %s", id, q)
		}
	}
	if got[101] != (LuckyBadge{Points: 73}) || got[103].Points != 58 {
		t.Errorf("값은 예전과 같아야 한다: %+v", got)
	}
	if _, ok := got[105]; ok {
		t.Errorf("기록 없는 글은 맵에 없다: %+v", got)
	}

	// 워터마크는 캐시된다 — 다음 조회는 워터마크를 다시 읽지 않는다.
	counter.reset()
	if _, err := LuckyBadgesByWrID(db, "free", []int{101}, LuckyTierNames{}); err != nil {
		t.Fatalf("조회 실패: %v", err)
	}
	if n := countSQL(counter.snapshot(), "MAX(CAST"); n != 0 {
		t.Errorf("워터마크 캐시 적중이면 다시 읽지 않는다: %v", counter.snapshot())
	}
}

// TestLuckyPointGate_NeitherSkipsPointQuery — 요청 id 가 모두 워터마크 초과·원장 밖이면 포인트 조회가 없다.
func TestLuckyPointGate_NeitherSkipsPointQuery(t *testing.T) {
	db, counter := newLuckyBadgeTestDB(t, true)
	warmLuckyWatermarkForTest(t, db, "free")
	counter.reset()
	got, err := LuckyBadgesByWrID(db, "free", []int{105, 110, 200}, LuckyTierNames{})
	if err != nil {
		t.Fatalf("조회 실패: %v", err)
	}
	sqls := counter.snapshot()
	if q := luckyPointQueryOf(sqls); q != "" {
		t.Errorf("포인트 조회가 없어야 한다: %s", q)
	}
	if n := countSQL(sqls, "g5_na_xp"); n != 0 {
		t.Errorf("경험치 조회도 없어야 한다: %v", sqls)
	}
	if len(got) != 0 {
		t.Errorf("기록 없음: %+v", got)
	}

	// 캐시가 찬 뒤에는 0쿼리다.
	counter.reset()
	if _, err := LuckyBadgesByWrID(db, "free", []int{105, 110}, LuckyTierNames{}); err != nil {
		t.Fatalf("조회 실패: %v", err)
	}
	if n := len(counter.snapshot()); n != 0 {
		t.Errorf("캐시 적중·대상 없음이면 0쿼리여야 한다: %v", counter.snapshot())
	}
}

// TestLuckyPointGate_NoLegacyBoardLedgerOnly — 원장 밖 행운 포인트가 없는 게시판(other)은 원장 id 만 조회한다.
func TestLuckyPointGate_NoLegacyBoardLedgerOnly(t *testing.T) {
	db, counter := newLuckyBadgeTestDB(t, true)
	warmLuckyWatermarkForTest(t, db, "other")
	counter.reset()
	got, err := LuckyBadgesByWrID(db, "other", []int{1, 50, 104}, LuckyTierNames{})
	if err != nil {
		t.Fatalf("조회 실패: %v", err)
	}
	q := luckyPointQueryOf(counter.snapshot())
	if !luckyPointQueryHas(q, "104") || luckyPointQueryHas(q, "1") || luckyPointQueryHas(q, "50") {
		t.Errorf("원장 id(104)만 조회해야 한다: %s", q)
	}
	if got[104].Points != 777 {
		t.Errorf("원장 글 포인트는 그대로: %+v", got[104])
	}

	counter.reset()
	if _, err := LuckyBadgesByWrID(db, "other", []int{1, 50}, LuckyTierNames{}); err != nil {
		t.Fatalf("조회 실패: %v", err)
	}
	if q := luckyPointQueryOf(counter.snapshot()); q != "" {
		t.Errorf("원장 밖 id 뿐이면 포인트 조회가 없어야 한다: %s", q)
	}
}

// TestLuckyPointGate_WatermarkErrorFailsOpen — 워터마크 조회가 실패하면 그 요청과 이후 요청은 요청 id 전부로
// 포인트를 조회하고(값 그대로), 실패는 캐시하지 않으며 luckyWatermarkRetryAfter 뒤에 다시 시도한다.
func TestLuckyPointGate_WatermarkErrorFailsOpen(t *testing.T) {
	db, counter := newLuckyBadgeTestDB(t, true)
	setLuckyWatermarkRetryForTest(t, time.Hour)
	// 원장 집합은 캐시에 넣어 두고 원장 테이블만 없앤다 → 원장은 캐시 적중, 워터마크 조회만 실패.
	luckyLedger.put("free", map[string]struct{}{"102": {}, "103": {}, "106": {}}, time.Now())
	if err := db.Exec(`DROP TABLE g5_da_lucky_grant`).Error; err != nil {
		t.Fatalf("원장 삭제 실패: %v", err)
	}
	for round := 0; round < 2; round++ {
		counter.reset()
		got, err := LuckyBadgesByWrID(db, "free", []int{101, 103, 105, 110}, LuckyTierNames{})
		if err != nil {
			t.Fatalf("워터마크 실패는 조회 에러가 아니다: %v", err)
		}
		luckyWatermarks.wait()
		sqls := counter.snapshot()
		q := luckyPointQueryOf(sqls)
		for _, id := range []string{"101", "103", "105", "110"} {
			if !luckyPointQueryHas(q, id) {
				t.Errorf("회차 %d: 워터마크를 모르면 요청 id 전부(%s 포함)로 조회해야 한다: %s", round, id, q)
			}
		}
		if got[101].Points != 73 || got[103].Points != 58 {
			t.Errorf("회차 %d: 값은 예전과 같아야 한다: %+v", round, got)
		}
		want := 1 // 첫 요청은 백그라운드 조회를 띄우고, 실패 직후(재시도 간격 안)에는 다시 띄우지 않는다
		if round == 1 {
			want = 0
		}
		if n := countSQL(sqls, "MAX(CAST"); n != want {
			t.Errorf("회차 %d: 워터마크 조회 %d회여야 한다, got %d", round, want, n)
		}
	}

	// 재시도 간격이 지나면 다시 시도한다(실패는 캐시하지 않는다).
	luckyWatermarkRetryAfter = 0
	counter.reset()
	_, _ = LuckyBadgesByWrID(db, "free", []int{101}, LuckyTierNames{})
	luckyWatermarks.wait()
	if n := countSQL(counter.snapshot(), "MAX(CAST"); n != 1 {
		t.Errorf("재시도 간격 뒤에는 다시 조회해야 한다, 워터마크 조회 %d회", n)
	}
}

// setLuckyWatermarkRetryForTest 는 워터마크 재시도 간격을 바꾸고 테스트 끝에 되돌린다.
func setLuckyWatermarkRetryForTest(t *testing.T, d time.Duration) {
	t.Helper()
	luckyWatermarks.wait()
	prev := luckyWatermarkRetryAfter
	luckyWatermarkRetryAfter = d
	t.Cleanup(func() {
		luckyWatermarks.wait()
		luckyWatermarkRetryAfter = prev
	})
}

// fakeWatermarkLoader 는 호출 수를 세고, release 가 닫힐 때까지 막혀 있는 워터마크 로더다.
type fakeWatermarkLoader struct {
	calls   atomic.Int32
	mu      sync.Mutex
	release chan struct{}
	once    *sync.Once
	wm      luckyWatermark
	err     error
}

// installFakeWatermarkLoader 는 가짜 로더를 끼우고, 테스트 끝에 풀어 준 뒤(막힌 조회가 끝나게) 원래 로더로 돌린다.
func installFakeWatermarkLoader(t *testing.T, wm luckyWatermark, err error) *fakeWatermarkLoader {
	t.Helper()
	luckyWatermarks.reset()
	f := &fakeWatermarkLoader{release: make(chan struct{}), once: &sync.Once{}, wm: wm, err: err}
	prev := luckyWatermarkLoad
	luckyWatermarkLoad = f.load
	t.Cleanup(func() {
		f.open()
		luckyWatermarks.reset()
		luckyWatermarkLoad = prev
	})
	return f
}

func (f *fakeWatermarkLoader) load(_ context.Context, _ *gorm.DB, _ string) (luckyWatermark, error) {
	f.calls.Add(1)
	f.mu.Lock()
	release := f.release
	f.mu.Unlock()
	<-release
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.wm, f.err
}

// open 은 막힌(그리고 앞으로 올) 조회를 풀어 준다.
func (f *fakeWatermarkLoader) open() {
	f.mu.Lock()
	release, once := f.release, f.once
	f.mu.Unlock()
	once.Do(func() { close(release) })
}

// rearm 은 다음 조회부터 다시 막는다(돌고 있는 조회가 없을 때 부른다).
func (f *fakeWatermarkLoader) rearm() {
	f.mu.Lock()
	f.release, f.once = make(chan struct{}), &sync.Once{}
	f.mu.Unlock()
}

func (f *fakeWatermarkLoader) set(wm luckyWatermark, err error) {
	f.mu.Lock()
	f.wm, f.err = wm, err
	f.mu.Unlock()
}

// TestLuckyPointGate_MissDoesNotBlockAndLoadsOnce — 워터마크가 없으면 요청은 기다리지 않고 「모름」(요청 id 전부)을
// 받고, 동시에 몰려도 백그라운드 조회는 게시판당 하나다. 조회가 끝나면 다음 요청부터 그 값으로 거른다.
func TestLuckyPointGate_MissDoesNotBlockAndLoadsOnce(t *testing.T) {
	f := installFakeWatermarkLoader(t, luckyWatermark{maxID: 101, has: true}, nil)
	ids := []string{"101", "103", "105"}
	ledger := map[string]struct{}{"103": {}}

	const n = 32
	results := make(chan []string, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- luckyPointGate(nil, "free", ids, ledger, nil)
		}()
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("워터마크 조회를 기다리면 안 된다(로더가 막혀 있는 동안 요청이 끝나야 한다)")
	}
	close(results)
	for got := range results {
		if strings.Join(got, ",") != "101,103,105" {
			t.Errorf("모르는 동안은 요청 id 전부여야 한다: %v", got)
		}
	}

	f.open()
	luckyWatermarks.wait()
	if c := f.calls.Load(); c != 1 {
		t.Errorf("백그라운드 조회는 게시판당 하나여야 한다, got %d", c)
	}
	if got := luckyPointGate(nil, "free", ids, ledger, nil); strings.Join(got, ",") != "101,103" {
		t.Errorf("적재 뒤에는 워터마크 이하·원장 id 만: %v", got)
	}
	if c := f.calls.Load(); c != 1 {
		t.Errorf("적중이면 다시 조회하지 않는다, got %d", c)
	}
}

// TestLuckyPointGate_LoadErrorStaysUnknownThenRetries — 조회 실패는 캐시하지 않는다(「모름」 유지). 재시도 간격 안에는
// 다시 띄우지 않고, 지나면 다시 띄워 성공한 값을 쓴다.
func TestLuckyPointGate_LoadErrorStaysUnknownThenRetries(t *testing.T) {
	setLuckyWatermarkRetryForTest(t, time.Hour)
	f := installFakeWatermarkLoader(t, luckyWatermark{}, errors.New("timeout"))
	f.open()
	ids := []string{"101", "105"}

	if got := luckyPointGate(nil, "free", ids, map[string]struct{}{}, nil); len(got) != 2 {
		t.Errorf("모르면 전부: %v", got)
	}
	luckyWatermarks.wait()
	if got := luckyPointGate(nil, "free", ids, map[string]struct{}{}, nil); len(got) != 2 {
		t.Errorf("실패 뒤에도 「모름」 — 전부: %v", got)
	}
	luckyWatermarks.wait()
	if c := f.calls.Load(); c != 1 {
		t.Errorf("재시도 간격 안에는 다시 조회하지 않는다, got %d", c)
	}

	f.set(luckyWatermark{maxID: 101, has: true}, nil)
	luckyWatermarkRetryAfter = 0
	_ = luckyPointGate(nil, "free", ids, map[string]struct{}{}, nil) // 재시도를 띄운다
	luckyWatermarks.wait()
	if c := f.calls.Load(); c != 2 {
		t.Errorf("재시도 간격 뒤에는 다시 조회한다, got %d", c)
	}
	if got := luckyPointGate(nil, "free", ids, map[string]struct{}{}, nil); strings.Join(got, ",") != "101" {
		t.Errorf("재시도 성공 뒤에는 그 값으로: %v", got)
	}
}

// TestLuckyPointGate_ExpiredKeepsValueWhileRefreshing — 만료돼도 새 값이 올 때까지 이전 값을 쓰고(기다리지 않음),
// 뒤에서 하나만 다시 읽는다. 다시 읽기가 실패해도 이전 값을 그대로 쓴다.
func TestLuckyPointGate_ExpiredKeepsValueWhileRefreshing(t *testing.T) {
	setLuckyWatermarkRetryForTest(t, 0)
	f := installFakeWatermarkLoader(t, luckyWatermark{maxID: 101, has: true}, nil)
	f.open()
	luckyWatermarks.lookup(nil, "free", time.Now())
	luckyWatermarks.wait()

	later := time.Now().Add(luckyWatermarkTTL + time.Minute)
	f.set(luckyWatermark{}, errors.New("refresh failed"))
	f.rearm() // 다시 읽기를 막아 두고 그동안 요청을 보낸다
	for i := 0; i < 3; i++ {
		wm, ok := luckyWatermarks.lookup(nil, "free", later)
		if !ok || wm.maxID != 101 {
			f.open()
			t.Fatalf("만료 뒤에도 이전 값을 써야 한다: %+v %v", wm, ok)
		}
	}
	f.open()
	luckyWatermarks.wait()
	if c := f.calls.Load(); c != 2 {
		t.Errorf("만료 뒤 다시 읽기는 하나여야 한다(처음 1 + 다시 1), got %d", c)
	}
	if wm, ok := luckyWatermarks.lookup(nil, "free", later); !ok || wm.maxID != 101 {
		t.Errorf("다시 읽기 실패면 이전 값 유지: %+v %v", wm, ok)
	}
	luckyWatermarks.wait()

	f.set(luckyWatermark{maxID: 120, has: true}, nil)
	luckyWatermarks.lookup(nil, "free", later)
	luckyWatermarks.wait()
	if wm, ok := luckyWatermarks.lookup(nil, "free", time.Now()); !ok || wm.maxID != 120 {
		t.Errorf("다시 읽기 성공이면 새 값: %+v %v", wm, ok)
	}
}

// TestLuckyPointGate_FirstRequestFullThenGated — 실제 쿼리로: 첫 요청은 워터마크를 모르니 요청 id 전부로 조회하고,
// 백그라운드 조회가 끝난 뒤의 요청은 워터마크 이하·원장 id 로만 조회한다.
func TestLuckyPointGate_FirstRequestFullThenGated(t *testing.T) {
	db, counter := newLuckyBadgeTestDB(t, true)
	if _, err := LuckyBadgesByWrID(db, "free", []int{101, 105}, LuckyTierNames{}); err != nil {
		t.Fatalf("조회 실패: %v", err)
	}
	luckyWatermarks.wait()
	if q := luckyPointQueryOf(counter.snapshot()); !luckyPointQueryHas(q, "105") {
		t.Errorf("첫 요청은 전부 조회: %s", q)
	}
	counter.reset()
	got, err := LuckyBadgesByWrID(db, "free", []int{101, 105}, LuckyTierNames{})
	if err != nil {
		t.Fatalf("조회 실패: %v", err)
	}
	sqls := counter.snapshot()
	if q := luckyPointQueryOf(sqls); !luckyPointQueryHas(q, "101") || luckyPointQueryHas(q, "105") {
		t.Errorf("적재 뒤에는 워터마크 이하만: %s", q)
	}
	if n := countSQL(sqls, "MAX(CAST"); n != 0 {
		t.Errorf("요청 안에서 워터마크를 읽지 않는다: %v", sqls)
	}
	if got[101].Points != 73 {
		t.Errorf("값은 예전과 같아야 한다: %+v", got)
	}
}

// TestLuckyPointGate_LedgerErrorFailsOpen — 원장 조회가 실패하면 포인트도 요청 id 전부로 조회한다.
func TestLuckyPointGate_LedgerErrorFailsOpen(t *testing.T) {
	db, counter := newLuckyBadgeTestDB(t, true)
	if err := db.Exec(`DROP TABLE g5_da_lucky_grant`).Error; err != nil {
		t.Fatalf("원장 삭제 실패: %v", err)
	}
	counter.reset()
	got, err := LuckyBadgesByWrID(db, "free", []int{101, 105, 110}, LuckyTierNames{})
	if err != nil {
		t.Fatalf("원장 실패는 조회 에러가 아니다: %v", err)
	}
	q := luckyPointQueryOf(counter.snapshot())
	for _, id := range []string{"101", "105", "110"} {
		if !luckyPointQueryHas(q, id) {
			t.Errorf("원장 실패면 요청 id 전부(%s 포함)로 조회해야 한다: %s", id, q)
		}
	}
	if got[101].Points != 73 {
		t.Errorf("값은 예전과 같아야 한다: %+v", got)
	}
}

// TestLuckyPointGate_NonNumericLegacyRelID — 숫자가 아닌 옛 rel_id 는 워터마크를 올리지 않고 조회를 깨지 않는다.
func TestLuckyPointGate_NonNumericLegacyRelID(t *testing.T) {
	db, counter := newLuckyBadgeTestDB(t, true)
	if err := db.Exec(`INSERT INTO g5_point (mb_id, po_datetime, po_content, po_point, po_rel_table, po_rel_id, po_rel_action) VALUES
		('m7', '2026-01-01 00:00:00', '나리야 럭키 포인트', 5, 'free', 'abc', '@lucky')`).Error; err != nil {
		t.Fatalf("시드 실패: %v", err)
	}
	warmLuckyWatermarkForTest(t, db, "free")
	counter.reset()
	got, err := LuckyBadgesByWrID(db, "free", []int{101, 105}, LuckyTierNames{})
	if err != nil {
		t.Fatalf("조회 실패: %v", err)
	}
	q := luckyPointQueryOf(counter.snapshot())
	if !luckyPointQueryHas(q, "101") || luckyPointQueryHas(q, "105") {
		t.Errorf("워터마크는 101 그대로여야 한다: %s", q)
	}
	if got[101].Points != 73 {
		t.Errorf("값은 예전과 같아야 한다: %+v", got)
	}
}
