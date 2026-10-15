package main

import (
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ⛔ 이 파일이 지키는 계약 (글 상세 처리 상태 조회 게이트):
//
//   - attachPostStatus 는 게시판별 상태 맵에서 꺼낸다. 행이 있는 글만 status / status_updated_at 이 붙고,
//     없는 글은 지금처럼 키가 생기지 않는다. 맵은 TTL 안에서 게시판당 한 번만 읽는다.
//   - 맵 조회가 실패하면 캐시하지 않고 단건 조회로 대신한다(fail-open — 상세를 막지 않는다).
//   - upsertPostStatus·clearPostStatus 가 성공하면 그 게시판 맵만 비운다. 실패하면 비우지 않는다.
//   - 목록(enrichWithPostStatus)도 같은 맵에서 꺼낸다. 맵이 적중하면 목록 묶음(IN) 쿼리를 하지 않고,
//     맵 조회가 실패하면 예전 묶음 쿼리로 대신한다. 붙는 키(status·status_updated_at)는 상세와 같다.
//
// ⚠️ 한계: sqlite 에는 DATE_FORMAT·CONVERT_TZ 가 없어 실제 맵 적재 SQL 의 결과는 여기서 못 본다.
// 맵 적재와 단건 조회는 같은 컬럼 상수(postStatusSelect)를 쓴다 — 결과 동일성은 코드 리뷰와 운영 DB 대조가 맡는다.

func newPostStatusTestDB(t *testing.T, withTable bool) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("sqlite 열기 실패: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql.DB 얻기 실패: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if withTable {
		if err := db.Exec(`CREATE TABLE g5_da_post_status (
			board_id TEXT NOT NULL, wr_id INTEGER NOT NULL, status TEXT NOT NULL,
			set_by TEXT, source TEXT, note TEXT, created_at DATETIME, updated_at DATETIME,
			PRIMARY KEY (board_id, wr_id))`).Error; err != nil {
			t.Fatalf("테이블 생성 실패: %v", err)
		}
	}
	postStatusBoards.reset()
	t.Cleanup(postStatusBoards.reset)
	return db
}

// stubPostStatusLoader 는 맵 적재를 바꿔 끼우고 호출 횟수(게시판별)를 센다.
func stubPostStatusLoader(t *testing.T, fn func(slug string) (map[int]postStatusRow, error)) map[string]int {
	t.Helper()
	calls := map[string]int{}
	prev := postStatusBoardLoader
	postStatusBoardLoader = func(_ *gorm.DB, slug string) (map[int]postStatusRow, error) {
		calls[slug]++
		return fn(slug)
	}
	t.Cleanup(func() { postStatusBoardLoader = prev })
	return calls
}

func TestAttachPostStatus_FromBoardMap(t *testing.T) {
	db := newPostStatusTestDB(t, false)
	calls := stubPostStatusLoader(t, func(slug string) (map[int]postStatusRow, error) {
		if slug == "bug" {
			return map[int]postStatusRow{5: {WrID: 5, Status: "resolved", UpdatedKST: "2026-01-02 03:04:05"}}, nil
		}
		return map[int]postStatusRow{}, nil
	})

	hit := map[string]any{}
	attachPostStatus(db, "bug", 5, hit)
	if hit["status"] != "resolved" || hit["status_updated_at"] != "2026-01-02 03:04:05" {
		t.Fatalf("맵의 값이 그대로 붙어야 한다: %v", hit)
	}

	miss := map[string]any{}
	attachPostStatus(db, "bug", 6, miss)
	if _, ok := miss["status"]; ok {
		t.Fatalf("행 없는 글에 키가 생겼다: %v", miss)
	}
	if _, ok := miss["status_updated_at"]; ok {
		t.Fatalf("행 없는 글에 키가 생겼다: %v", miss)
	}

	other := map[string]any{}
	attachPostStatus(db, "free", 5, other)
	if len(other) != 0 {
		t.Fatalf("다른 게시판 같은 wr_id 가 섞였다: %v", other)
	}

	if calls["bug"] != 1 || calls["free"] != 1 {
		t.Fatalf("TTL 안에서는 게시판당 한 번만 읽어야 한다: %v", calls)
	}
}

func TestAttachPostStatus_LoadFailureFallsBackAndIsNotCached(t *testing.T) {
	// 테이블이 없어 단건 조회도 실패한다 — 상세는 막히지 않고 키만 생략된다.
	db := newPostStatusTestDB(t, false)
	calls := stubPostStatusLoader(t, func(string) (map[int]postStatusRow, error) {
		return nil, errors.New("boom")
	})

	for i := 0; i < 2; i++ {
		detail := map[string]any{"id": 5}
		attachPostStatus(db, "bug", 5, detail)
		if _, ok := detail["status"]; ok {
			t.Fatalf("조회가 모두 실패했는데 status 가 붙었다: %v", detail)
		}
	}
	if calls["bug"] != 2 {
		t.Fatalf("실패는 캐시하지 않아야 한다(매번 재시도): %v", calls)
	}
}

func TestClearPostStatus_InvalidatesBoardMap(t *testing.T) {
	db := newPostStatusTestDB(t, true)
	calls := stubPostStatusLoader(t, func(string) (map[int]postStatusRow, error) {
		return map[int]postStatusRow{}, nil
	})

	attachPostStatus(db, "bug", 5, map[string]any{})
	attachPostStatus(db, "free", 5, map[string]any{})
	if err := clearPostStatus(db, "bug", 5); err != nil {
		t.Fatalf("clearPostStatus 실패: %v", err)
	}
	attachPostStatus(db, "bug", 5, map[string]any{})
	attachPostStatus(db, "free", 5, map[string]any{})
	if calls["bug"] != 2 {
		t.Fatalf("해제 뒤에는 그 게시판 맵을 다시 읽어야 한다: %v", calls)
	}
	if calls["free"] != 1 {
		t.Fatalf("다른 게시판 맵은 그대로여야 한다: %v", calls)
	}
}

func TestPostStatusWriteFailure_DoesNotInvalidate(t *testing.T) {
	db := newPostStatusTestDB(t, false) // 테이블 없음 — 쓰기가 실패한다
	calls := stubPostStatusLoader(t, func(string) (map[int]postStatusRow, error) {
		return map[int]postStatusRow{}, nil
	})

	attachPostStatus(db, "bug", 5, map[string]any{})
	if err := clearPostStatus(db, "bug", 5); err == nil {
		t.Fatal("테이블이 없는데 clearPostStatus 가 성공했다")
	}
	if err := upsertPostStatus(db, "bug", 5, "resolved", "admin", "admin", ""); err == nil {
		t.Fatal("테이블이 없는데 upsertPostStatus 가 성공했다")
	}
	attachPostStatus(db, "bug", 5, map[string]any{})
	if calls["bug"] != 1 {
		t.Fatalf("쓰기가 실패하면 맵을 비우지 않는다: %v", calls)
	}
}

func TestPostStatusBoardCache_TTLAndGeneration(t *testing.T) {
	c := &postStatusBoardCache{entries: map[string]postStatusBoardEntry{}, gens: map[string]uint64{}}
	now := time.Now()
	byID := map[int]postStatusRow{1: {WrID: 1, Status: "hold"}}

	_, gen, ok := c.get("bug", now)
	if ok {
		t.Fatal("빈 캐시가 적중했다")
	}
	c.put("bug", byID, gen, now)
	if _, _, ok := c.get("bug", now.Add(postStatusBoardTTL-time.Second)); !ok {
		t.Fatal("TTL 안에서는 적중해야 한다")
	}
	if _, _, ok := c.get("bug", now.Add(postStatusBoardTTL)); ok {
		t.Fatal("TTL 이 지나면 미스여야 한다")
	}

	// 조회 도중 무효화가 끼어들면 그 결과는 넣지 않는다.
	_, gen, _ = c.get("bug", now.Add(postStatusBoardTTL))
	c.forget("bug")
	c.put("bug", byID, gen, now)
	if _, _, ok := c.get("bug", now); ok {
		t.Fatal("무효화 전 세대의 조회 결과가 적재됐다")
	}
	// 다른 게시판의 세대는 영향받지 않는다.
	_, gen, _ = c.get("free", now)
	c.forget("bug")
	c.put("free", byID, gen, now)
	if _, _, ok := c.get("free", now); !ok {
		t.Fatal("다른 게시판 무효화가 이 게시판 적재를 막았다")
	}
}

// countPostStatusQueries 는 상태 테이블로 나간 조회 수와 마지막 SQL 을 센다.
func countPostStatusQueries(t *testing.T, db *gorm.DB) (*int, *string) {
	t.Helper()
	n := 0
	last := ""
	if err := db.Callback().Query().After("gorm:query").Register("test:count_post_status", func(tx *gorm.DB) {
		if tx.Statement.Table == postStatusTable {
			n++
			last = tx.Statement.SQL.String()
		}
	}); err != nil {
		t.Fatalf("콜백 등록 실패: %v", err)
	}
	return &n, &last
}

func TestEnrichWithPostStatus_FromBoardMap(t *testing.T) {
	db := newPostStatusTestDB(t, true)
	queries, _ := countPostStatusQueries(t, db)
	calls := stubPostStatusLoader(t, func(slug string) (map[int]postStatusRow, error) {
		if slug == "bug" {
			return map[int]postStatusRow{
				5: {WrID: 5, Status: "resolved", UpdatedKST: "2026-01-02 03:04:05"},
				7: {WrID: 7, Status: "hold", UpdatedKST: "2026-01-03 00:00:00"},
			}, nil
		}
		return map[int]postStatusRow{}, nil
	})

	items := []map[string]any{{"id": 5}, {"id": int64(6)}, {"id": float64(7)}, {"title": "no id"}}
	items = enrichWithPostStatus(db, "bug", items)
	if items[0]["status"] != "resolved" || items[0]["status_updated_at"] != "2026-01-02 03:04:05" {
		t.Fatalf("맵의 값이 그대로 붙어야 한다: %v", items[0])
	}
	if items[2]["status"] != "hold" || items[2]["status_updated_at"] != "2026-01-03 00:00:00" {
		t.Fatalf("맵의 값이 그대로 붙어야 한다: %v", items[2])
	}
	for _, i := range []int{1, 3} {
		if _, ok := items[i]["status"]; ok {
			t.Fatalf("행 없는 글에 키가 생겼다: %v", items[i])
		}
		if _, ok := items[i]["status_updated_at"]; ok {
			t.Fatalf("행 없는 글에 키가 생겼다: %v", items[i])
		}
	}

	// 상세와 같은 맵을 쓴다 — 상세가 다시 읽지 않는다.
	detail := map[string]any{}
	attachPostStatus(db, "bug", 5, detail)
	if detail["status"] != items[0]["status"] || detail["status_updated_at"] != items[0]["status_updated_at"] {
		t.Fatalf("상세와 목록의 값이 다르다: detail=%v list=%v", detail, items[0])
	}
	enrichWithPostStatus(db, "bug", []map[string]any{{"id": 5}})
	if calls["bug"] != 1 {
		t.Fatalf("TTL 안에서는 게시판당 한 번만 읽어야 한다: %v", calls)
	}
	if *queries != 0 {
		t.Fatalf("맵 적중 시 목록 묶음 쿼리가 나가면 안 된다: %d", *queries)
	}
}

func TestEnrichWithPostStatus_EmptyBoardMap(t *testing.T) {
	db := newPostStatusTestDB(t, true)
	queries, _ := countPostStatusQueries(t, db)
	calls := stubPostStatusLoader(t, func(string) (map[int]postStatusRow, error) {
		return map[int]postStatusRow{}, nil
	})

	for i := 0; i < 2; i++ {
		items := enrichWithPostStatus(db, "free", []map[string]any{{"id": 1}, {"id": 2}})
		for _, item := range items {
			if len(item) != 1 {
				t.Fatalf("상태 없는 게시판 목록에 키가 생겼다: %v", item)
			}
		}
	}
	if calls["free"] != 1 {
		t.Fatalf("빈 맵도 캐시되어야 한다: %v", calls)
	}
	if *queries != 0 {
		t.Fatalf("빈 맵 적중 시 목록 묶음 쿼리가 나가면 안 된다: %d", *queries)
	}
}

func TestEnrichWithPostStatus_LoadFailureFallsBackToInQuery(t *testing.T) {
	// sqlite 에는 DATE_FORMAT 이 없어 묶음 쿼리 자체도 실패한다 — 쿼리가 나갔는지와 목록이 막히지 않는지만 본다.
	db := newPostStatusTestDB(t, true)
	queries, last := countPostStatusQueries(t, db)
	calls := stubPostStatusLoader(t, func(string) (map[int]postStatusRow, error) {
		return nil, errors.New("boom")
	})

	items := enrichWithPostStatus(db, "bug", []map[string]any{{"id": 5}, {"id": 6}})
	if len(items) != 2 {
		t.Fatalf("목록 항목이 바뀌었다: %v", items)
	}
	if *queries != 1 {
		t.Fatalf("맵 실패 시 목록 묶음 쿼리 1회로 대신해야 한다: %d", *queries)
	}
	if !strings.Contains(*last, "wr_id IN") {
		t.Fatalf("대체 경로가 묶음(IN) 쿼리가 아니다: %s", *last)
	}
	for _, item := range items {
		if _, ok := item["status"]; ok {
			t.Fatalf("조회가 모두 실패했는데 status 가 붙었다: %v", item)
		}
	}

	enrichWithPostStatus(db, "bug", []map[string]any{{"id": 5}})
	if calls["bug"] != 2 {
		t.Fatalf("실패는 캐시하지 않아야 한다(매번 재시도): %v", calls)
	}
}
