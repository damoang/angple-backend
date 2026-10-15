package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// fakeGivingListCache is an in-memory GivingListCache for tests.
type fakeGivingListCache struct {
	store   map[string][]byte
	gets    int
	sets    int
	lastTTL time.Duration
	deleted []string
}

func newFakeGivingListCache() *fakeGivingListCache {
	return &fakeGivingListCache{store: map[string][]byte{}}
}

func (f *fakeGivingListCache) Get(_ context.Context, key string, dest interface{}) error {
	f.gets++
	b, ok := f.store[key]
	if !ok {
		return errors.New("cache miss")
	}
	return json.Unmarshal(b, dest)
}

func (f *fakeGivingListCache) Set(_ context.Context, key string, value interface{}, ttl time.Duration) error {
	f.sets++
	f.lastTTL = ttl
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	f.store[key] = b
	return nil
}

func (f *fakeGivingListCache) Delete(_ context.Context, keys ...string) error {
	for _, k := range keys {
		f.deleted = append(f.deleted, k)
		delete(f.store, k)
	}
	return nil
}

func sampleGivingRows() []givingRow {
	return []givingRow{
		{WrID: 10, WrSubject: "첫 나눔", Wr10: "https://example.com/a.jpg", WrContent: "<p>본문</p>", ParticipantCount: 3},
		{WrID: 11, WrSubject: "둘째 나눔", Wr7: "2", ParticipantCount: 0},
	}
}

func countingLoader(rows []givingRow, calls *int) func() ([]givingRow, error) {
	return func() ([]givingRow, error) {
		*calls++
		return rows, nil
	}
}

func openGivingTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=private"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	// in-memory DB 는 연결마다 따로라서 연결 하나로 고정한다.
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func TestGivingListRowsCacheHitSkipsDB(t *testing.T) {
	cache := newFakeGivingListCache()
	want := sampleGivingRows()
	b, _ := json.Marshal(want)
	cache.store[givingListCacheKey] = b

	h := &GivingHandler{listCache: cache}
	calls := 0
	got, err := h.givingListRows(context.Background(), countingLoader(nil, &calls))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 0 {
		t.Fatalf("loader called %d times on cache hit, want 0", calls)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestGivingListRowsCacheMissStores(t *testing.T) {
	cache := newFakeGivingListCache()
	h := &GivingHandler{listCache: cache}
	rows := sampleGivingRows()
	calls := 0
	load := countingLoader(rows, &calls)

	got, err := h.givingListRows(context.Background(), load)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 || !reflect.DeepEqual(got, rows) {
		t.Fatalf("first call: calls=%d got=%+v", calls, got)
	}
	if _, ok := cache.store[givingListCacheKey]; !ok || cache.sets != 1 {
		t.Fatalf("rows not stored in cache (sets=%d)", cache.sets)
	}
	if cache.lastTTL != givingListCacheTTL {
		t.Fatalf("ttl = %v, want %v", cache.lastTTL, givingListCacheTTL)
	}

	got, err = h.givingListRows(context.Background(), load)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("loader called again on second request (calls=%d)", calls)
	}
	if !reflect.DeepEqual(got, rows) {
		t.Fatalf("cached rows differ: got %+v, want %+v", got, rows)
	}
}

func TestGivingListRowsCorruptCacheFallsBackToDB(t *testing.T) {
	cache := newFakeGivingListCache()
	cache.store[givingListCacheKey] = []byte("not-json")
	h := &GivingHandler{listCache: cache}
	rows := sampleGivingRows()
	calls := 0

	got, err := h.givingListRows(context.Background(), countingLoader(rows, &calls))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 || !reflect.DeepEqual(got, rows) {
		t.Fatalf("expected DB fallback: calls=%d got=%+v", calls, got)
	}
}

func TestGivingListRowsNilCache(t *testing.T) {
	h := &GivingHandler{}
	rows := sampleGivingRows()
	calls := 0
	load := countingLoader(rows, &calls)
	for i := 0; i < 2; i++ {
		got, err := h.givingListRows(context.Background(), load)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !reflect.DeepEqual(got, rows) {
			t.Fatalf("got %+v, want %+v", got, rows)
		}
	}
	if calls != 2 {
		t.Fatalf("loader calls = %d, want 2 without cache", calls)
	}
	// 캐시가 없어도 무효화는 아무 일 없이 끝나야 한다.
	h.invalidateGivingListCache()
}

func TestGivingListRowsLoadErrorNotCached(t *testing.T) {
	cache := newFakeGivingListCache()
	h := &GivingHandler{listCache: cache}
	_, err := h.givingListRows(context.Background(), func() ([]givingRow, error) {
		return nil, errors.New("db down")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if cache.sets != 0 {
		t.Fatalf("error result must not be cached (sets=%d)", cache.sets)
	}
}

func TestGivingListHandlerServesFromCache(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cache := newFakeGivingListCache()
	b, _ := json.Marshal([]givingRow{
		{WrID: 10, WrSubject: "첫 나눔", Wr10: "https://example.com/a.jpg", ParticipantCount: 3},
	})
	cache.store[givingListCacheKey] = b

	// db 가 nil 이므로 DB 를 건드리면 패닉 — 캐시 적중 시 DB 미호출을 보장한다.
	h := &GivingHandler{listCache: cache}
	r := gin.New()
	r.GET("/list", h.List)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/list?tab=active", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Success bool             `json:"success"`
		Data    []GivingListItem `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Success || len(resp.Data) != 1 {
		t.Fatalf("unexpected response: %s", w.Body.String())
	}
	if resp.Data[0].ID != 10 || resp.Data[0].ParticipantCount != 3 || resp.Data[0].Extra10 != "https://example.com/a.jpg" {
		t.Fatalf("unexpected item: %+v", resp.Data[0])
	}
}

func TestAttachGivingParticipantCounts(t *testing.T) {
	db := openGivingTestDB(t)
	if err := db.Exec(`CREATE TABLE g5_giving_bid (bid_id INTEGER PRIMARY KEY AUTOINCREMENT, bo_table TEXT NOT NULL DEFAULT '', wr_id INTEGER NOT NULL, mb_id TEXT NOT NULL, bid_status INTEGER NOT NULL DEFAULT 1)`).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	// wr 1: a 두 번(중복) + b(취소 상태) → 종전 서브쿼리처럼 상태 무관 distinct 2
	// wr 2: c → 1, wr 3: 없음 → 0, wr 99: 목록 밖 → 무시
	if err := db.Exec(`INSERT INTO g5_giving_bid (bo_table, wr_id, mb_id, bid_status) VALUES
		('giving', 1, 'a', 1), ('giving', 1, 'a', 1), ('giving', 1, 'b', 0),
		('giving', 2, 'c', 1), ('giving', 99, 'z', 1)`).Error; err != nil {
		t.Fatalf("insert: %v", err)
	}

	h := &GivingHandler{db: db}
	rows := []givingRow{{WrID: 1}, {WrID: 2}, {WrID: 3, ParticipantCount: 7}}
	if err := h.attachGivingParticipantCounts(rows); err != nil {
		t.Fatalf("attach: %v", err)
	}
	want := map[int]int{1: 2, 2: 1, 3: 0}
	for _, r := range rows {
		if r.ParticipantCount != want[r.WrID] {
			t.Fatalf("wr_id %d: count = %d, want %d", r.WrID, r.ParticipantCount, want[r.WrID])
		}
	}
}

func TestAttachGivingParticipantCountsEmptySkipsQuery(t *testing.T) {
	// db 가 nil — 쿼리하면 패닉이므로 빈 목록에서 쿼리를 생략하는지 확인된다.
	h := &GivingHandler{}
	if err := h.attachGivingParticipantCounts(nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGivingAdminActionInvalidatesListCache(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openGivingTestDB(t)
	stmts := []string{
		`CREATE TABLE g5_write_giving (wr_id INTEGER PRIMARY KEY, mb_id TEXT NOT NULL DEFAULT '', wr_subject TEXT NOT NULL DEFAULT '', wr_2 TEXT NOT NULL DEFAULT '', wr_4 TEXT NOT NULL DEFAULT '', wr_5 TEXT NOT NULL DEFAULT '', wr_7 TEXT NOT NULL DEFAULT '', wr_8 TEXT NOT NULL DEFAULT '', wr_is_comment INTEGER NOT NULL DEFAULT 0)`,
		`CREATE TABLE g5_giving_meta (wr_id INTEGER PRIMARY KEY, status TEXT NOT NULL DEFAULT '', updated_at DATETIME)`,
		`INSERT INTO g5_write_giving (wr_id, mb_id, wr_subject) VALUES (5, 'host', '나눔')`,
		`INSERT INTO g5_giving_meta (wr_id, status) VALUES (5, 'open')`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("setup %q: %v", s, err)
		}
	}

	for _, action := range []string{"pause", "resume"} {
		cache := newFakeGivingListCache()
		cache.store[givingListCacheKey] = []byte("[]")
		h := &GivingHandler{db: db, listCache: cache}

		r := gin.New()
		r.POST("/admin/:id/:action", func(c *gin.Context) {
			c.Set("username", "host")
			h.AdminAction(c)
		})
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/admin/5/"+action, nil))

		if w.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, body=%s", action, w.Code, w.Body.String())
		}
		if len(cache.deleted) != 1 || cache.deleted[0] != givingListCacheKey {
			t.Fatalf("%s: deleted = %v, want [%s]", action, cache.deleted, givingListCacheKey)
		}
		if _, ok := cache.store[givingListCacheKey]; ok {
			t.Fatalf("%s: cache entry still present", action)
		}
	}
}

func TestGivingAdminActionUnknownDoesNotInvalidate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openGivingTestDB(t)
	if err := db.Exec(`CREATE TABLE g5_write_giving (wr_id INTEGER PRIMARY KEY, mb_id TEXT NOT NULL DEFAULT '', wr_subject TEXT NOT NULL DEFAULT '', wr_2 TEXT NOT NULL DEFAULT '', wr_4 TEXT NOT NULL DEFAULT '', wr_5 TEXT NOT NULL DEFAULT '', wr_7 TEXT NOT NULL DEFAULT '', wr_is_comment INTEGER NOT NULL DEFAULT 0)`).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := db.Exec(`INSERT INTO g5_write_giving (wr_id, mb_id) VALUES (5, 'host')`).Error; err != nil {
		t.Fatalf("insert: %v", err)
	}
	cache := newFakeGivingListCache()
	h := &GivingHandler{db: db, listCache: cache}
	r := gin.New()
	r.POST("/admin/:id/:action", func(c *gin.Context) {
		c.Set("username", "host")
		h.AdminAction(c)
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/admin/5/bogus", nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", w.Code)
	}
	if len(cache.deleted) != 0 {
		t.Fatalf("unexpected invalidation: %v", cache.deleted)
	}
}
