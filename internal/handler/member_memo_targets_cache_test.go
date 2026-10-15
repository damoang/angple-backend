package handler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// fakeMemoTargetsStore 는 MemoTargetsStore 의 메모리 구현이다.
type fakeMemoTargetsStore struct {
	mu     sync.Mutex
	data   map[string]string
	ttl    map[string]time.Duration
	getErr error
	setErr error
	delErr error
	sets   int
	dels   []string
}

func newFakeMemoTargetsStore() *fakeMemoTargetsStore {
	return &fakeMemoTargetsStore{data: map[string]string{}, ttl: map[string]time.Duration{}}
}

func (f *fakeMemoTargetsStore) Get(_ context.Context, key string) *redis.StringCmd {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getErr != nil {
		return redis.NewStringResult("", f.getErr)
	}
	v, ok := f.data[key]
	if !ok {
		return redis.NewStringResult("", redis.Nil)
	}
	return redis.NewStringResult(v, nil)
}

func (f *fakeMemoTargetsStore) Set(_ context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sets++
	if f.setErr != nil {
		return redis.NewStatusResult("", f.setErr)
	}
	switch v := value.(type) {
	case []byte:
		f.data[key] = string(v)
	case string:
		f.data[key] = v
	}
	f.ttl[key] = expiration
	return redis.NewStatusResult("OK", nil)
}

func (f *fakeMemoTargetsStore) Del(_ context.Context, keys ...string) *redis.IntCmd {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dels = append(f.dels, keys...)
	if f.delErr != nil {
		return redis.NewIntResult(0, f.delErr)
	}
	for _, k := range keys {
		delete(f.data, k)
	}
	return redis.NewIntResult(int64(len(keys)), nil)
}

// snapshot 은 잠금 아래에서 저장 상태를 복사한다(지연 삭제 고루틴과 경합하지 않게).
func (f *fakeMemoTargetsStore) snapshot() (map[string]string, []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data := make(map[string]string, len(f.data))
	for k, v := range f.data {
		data[k] = v
	}
	return data, append([]string(nil), f.dels...)
}

func (f *fakeMemoTargetsStore) put(key, value string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data[key] = value
}

// stubMemoTargetsRetryDelay 는 지연 무효화 간격을 바꾼다.
func stubMemoTargetsRetryDelay(t *testing.T, d time.Duration) {
	t.Helper()
	prev := memoTargetsInvalidateRetryDelay
	memoTargetsInvalidateRetryDelay = d
	t.Cleanup(func() { memoTargetsInvalidateRetryDelay = prev })
}

// waitMemoTargetsDels 는 삭제 호출이 want 건 이상 쌓일 때까지 기다린다.
func waitMemoTargetsDels(t *testing.T, f *fakeMemoTargetsStore, want int) []string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		_, dels := f.snapshot()
		if len(dels) >= want {
			return dels
		}
		if time.Now().After(deadline) {
			t.Fatalf("dels = %v, want at least %d", dels, want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// newMemoGateNoTableDB 는 g5_member_memo 가 없는 DB 다 — 조회하면 오류가 난다.
func newMemoGateNoTableDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=private"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return db
}

func TestMemoTargetsCacheKey_PerViewerLowercase(t *testing.T) {
	if got := MemoTargetsCacheKey("Alice"); got != "memo:targets:alice" {
		t.Fatalf("key = %q", got)
	}
	if MemoTargetsCacheKey("alice") == MemoTargetsCacheKey("bob") {
		t.Fatal("keys must differ per viewer")
	}
}

func TestMemoTargetsCache_MissFillsFromDBAndStores(t *testing.T) {
	db := newMemoTargetsTestDB(t)
	insertMemo(t, db, "me", "a", "memo")
	insertMemo(t, db, "me", "blank", "") // 내용 없는 행도 게이트 집합에는 포함(교집합 조회에서 걸러진다)
	insertMemo(t, db, "other", "b", "memo")
	store := newFakeMemoTargetsStore()
	cache := NewMemoTargetsCacheWithStore(store)

	set, ok := cache.TargetSet(context.Background(), db, "me")
	if !ok {
		t.Fatal("miss must fill and return ok")
	}
	if _, has := set["a"]; !has {
		t.Fatalf("set must contain a, got %v", set)
	}
	if _, has := set["blank"]; !has {
		t.Fatalf("gate set must be a superset (blank memo row), got %v", set)
	}
	if _, has := set["b"]; has {
		t.Fatalf("other viewer's target leaked: %v", set)
	}
	key := MemoTargetsCacheKey("me")
	if _, stored := store.data[key]; !stored {
		t.Fatalf("set must be stored under %q, got keys %v", key, store.data)
	}
	if store.ttl[key] != MemoTargetsCacheTTL {
		t.Fatalf("ttl = %v", store.ttl[key])
	}
	if len(store.data) != 1 {
		t.Fatalf("only the viewer key must be written, got %v", store.data)
	}
}

func TestMemoTargetsCache_EmptySetIsCached(t *testing.T) {
	db := newMemoTargetsTestDB(t)
	store := newFakeMemoTargetsStore()
	cache := NewMemoTargetsCacheWithStore(store)

	set, ok := cache.TargetSet(context.Background(), db, "me")
	if !ok || len(set) != 0 {
		t.Fatalf("empty set must be ok, got %v %v", set, ok)
	}
	if got := store.data[MemoTargetsCacheKey("me")]; got != "[]" {
		t.Fatalf("stored = %q, want []", got)
	}
}

func TestMemoTargetsCache_HitDoesNotQueryDB(t *testing.T) {
	store := newFakeMemoTargetsStore()
	store.data[MemoTargetsCacheKey("me")] = `["x","y"]`
	cache := NewMemoTargetsCacheWithStore(store)

	set, ok := cache.TargetSet(context.Background(), newMemoGateNoTableDB(t), "me")
	if !ok || len(set) != 2 {
		t.Fatalf("hit must return cached set, got %v %v", set, ok)
	}
	if store.sets != 0 {
		t.Fatalf("hit must not write, sets=%d", store.sets)
	}
}

func TestMemoTargetsCache_KeysArePerViewer(t *testing.T) {
	store := newFakeMemoTargetsStore()
	store.data[MemoTargetsCacheKey("alice")] = `["x"]`
	db := newMemoTargetsTestDB(t)
	insertMemo(t, db, "bob", "y", "memo")
	cache := NewMemoTargetsCacheWithStore(store)

	set, ok := cache.TargetSet(context.Background(), db, "bob")
	if !ok {
		t.Fatal("bob must fill")
	}
	if _, has := set["x"]; has {
		t.Fatalf("bob must not read alice's set: %v", set)
	}
	if _, has := set["y"]; !has {
		t.Fatalf("bob set must contain y: %v", set)
	}
}

func TestMemoTargetsCache_RedisErrorIsUnknown(t *testing.T) {
	store := newFakeMemoTargetsStore()
	store.getErr = errors.New("timeout")
	cache := NewMemoTargetsCacheWithStore(store)

	db := newMemoTargetsTestDB(t)
	insertMemo(t, db, "me", "a", "memo")
	if set, ok := cache.TargetSet(context.Background(), db, "me"); ok {
		t.Fatalf("redis error must be unknown (fail-open), got %v", set)
	}
	if store.sets != 0 {
		t.Fatalf("redis error must not write, sets=%d", store.sets)
	}
}

func TestMemoTargetsCache_BadPayloadIsUnknown(t *testing.T) {
	for _, payload := range []string{"not-json", "null", `{"a":1}`} {
		store := newFakeMemoTargetsStore()
		store.data[MemoTargetsCacheKey("me")] = payload
		cache := NewMemoTargetsCacheWithStore(store)
		if set, ok := cache.TargetSet(context.Background(), newMemoGateNoTableDB(t), "me"); ok {
			t.Fatalf("payload %q must be unknown, got %v", payload, set)
		}
	}
}

func TestMemoTargetsCache_DBErrorIsUnknown(t *testing.T) {
	store := newFakeMemoTargetsStore()
	cache := NewMemoTargetsCacheWithStore(store)
	if set, ok := cache.TargetSet(context.Background(), newMemoGateNoTableDB(t), "me"); ok {
		t.Fatalf("db error must be unknown, got %v", set)
	}
	if store.sets != 0 {
		t.Fatalf("db error must not write, sets=%d", store.sets)
	}
}

func TestMemoTargetsCache_TruncatedIsNotCached(t *testing.T) {
	db := newMemoTargetsTestDB(t)
	if err := db.Exec(`INSERT INTO g5_member_memo (member_id, target_member_id, memo)
		WITH RECURSIVE seq(n) AS (SELECT 1 UNION ALL SELECT n + 1 FROM seq WHERE n < ?)
		SELECT 'me', 't' || n, 'memo' FROM seq`, MemoTargetsLimit+1).Error; err != nil {
		t.Fatalf("bulk insert: %v", err)
	}
	store := newFakeMemoTargetsStore()
	cache := NewMemoTargetsCacheWithStore(store)

	if _, ok := cache.TargetSet(context.Background(), db, "me"); ok {
		t.Fatal("truncated must be unknown")
	}
	if store.sets != 0 || len(store.data) != 0 {
		t.Fatalf("truncated must not be cached, sets=%d data=%d", store.sets, len(store.data))
	}
}

func TestMemoTargetsCache_SetErrorStillReturnsSet(t *testing.T) {
	db := newMemoTargetsTestDB(t)
	insertMemo(t, db, "me", "a", "memo")
	store := newFakeMemoTargetsStore()
	store.setErr = errors.New("oom")
	cache := NewMemoTargetsCacheWithStore(store)

	set, ok := cache.TargetSet(context.Background(), db, "me")
	if !ok {
		t.Fatal("set error must still return the DB result")
	}
	if _, has := set["a"]; !has {
		t.Fatalf("set must contain a: %v", set)
	}
}

func TestMemoTargetsCache_InvalidateDeletesViewerKey(t *testing.T) {
	stubMemoTargetsRetryDelay(t, time.Hour) // 이 테스트는 즉시 삭제만 본다
	store := newFakeMemoTargetsStore()
	store.data[MemoTargetsCacheKey("me")] = `["a"]`
	store.data[MemoTargetsCacheKey("other")] = `["b"]`
	cache := NewMemoTargetsCacheWithStore(store)

	cache.Invalidate(context.Background(), "Me")
	data, dels := store.snapshot()
	if _, still := data[MemoTargetsCacheKey("me")]; still {
		t.Fatal("viewer key must be deleted")
	}
	if _, kept := data[MemoTargetsCacheKey("other")]; !kept {
		t.Fatal("other viewer key must be kept")
	}
	if len(dels) != 1 {
		t.Fatalf("dels = %v", dels)
	}

	cache.Invalidate(context.Background(), "") // 빈 ID 는 무동작
	if _, dels := store.snapshot(); len(dels) != 1 {
		t.Fatalf("empty viewer must not delete, dels = %v", dels)
	}
}

func TestMemoTargetsCache_InvalidateDeletesAgainAfterDelay(t *testing.T) {
	stubMemoTargetsRetryDelay(t, 200*time.Millisecond)
	store := newFakeMemoTargetsStore()
	cache := NewMemoTargetsCacheWithStore(store)

	cache.Invalidate(context.Background(), "me")
	// 첫 삭제 뒤, 경합한 채우기가 쓰기 전 집합을 저장한 상황.
	store.put(MemoTargetsCacheKey("me"), `["stale"]`)

	dels := waitMemoTargetsDels(t, store, 2)
	key := MemoTargetsCacheKey("me")
	if dels[0] != key || dels[1] != key {
		t.Fatalf("dels = %v, want [%s %s]", dels, key, key)
	}
	data, _ := store.snapshot()
	if _, still := data[key]; still {
		t.Fatal("delayed delete must remove the stale set")
	}
}

func TestMemoTargetsCache_InvalidateErrorsAreLoggedOnly(t *testing.T) {
	stubMemoTargetsRetryDelay(t, 10*time.Millisecond)
	store := newFakeMemoTargetsStore()
	store.delErr = errors.New("down")
	cache := NewMemoTargetsCacheWithStore(store)

	cache.Invalidate(context.Background(), "me") // 패닉 없이 로그만
	waitMemoTargetsDels(t, store, 2)
}

func TestMemoTargetsCache_FillUsesWriter(t *testing.T) {
	db := newMemoTargetsTestDB(t)
	insertMemo(t, db, "me", "a", "memo")
	var sawWriter, sawQuery bool
	if err := db.Callback().Query().Before("gorm:query").Register("test:memo_gate_writer", func(tx *gorm.DB) {
		if tx.Statement.Table != "g5_member_memo" {
			return
		}
		sawQuery = true
		_, sawWriter = tx.Statement.Settings.Load("gorm:db_resolver:write")
	}); err != nil {
		t.Fatalf("register callback: %v", err)
	}
	cache := NewMemoTargetsCacheWithStore(newFakeMemoTargetsStore())

	if _, ok := cache.TargetSet(context.Background(), db, "me"); !ok {
		t.Fatal("fill must succeed")
	}
	if !sawQuery || !sawWriter {
		t.Fatalf("fill query must be pinned to writer (query=%v writer=%v)", sawQuery, sawWriter)
	}
}

func TestMemoTargetsCache_NilIsSafe(t *testing.T) {
	var cache *MemoTargetsCache
	if _, ok := cache.TargetSet(context.Background(), newMemoGateNoTableDB(t), "me"); ok {
		t.Fatal("nil cache must be unknown")
	}
	cache.Invalidate(context.Background(), "me")
	if NewMemoTargetsCache(nil) != nil {
		t.Fatal("nil client must yield nil cache")
	}
	if NewMemoTargetsCacheWithStore(nil) != nil {
		t.Fatal("nil store must yield nil cache")
	}
}
