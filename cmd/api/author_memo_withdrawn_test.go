package main

import (
	"context"
	"errors"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// enrichWithAuthorMemo 의 탈퇴 작성자 가드 계약:
//
//   - 탈퇴(숙려 포함) 작성자: author_memo 는 nil(메모 없음과 같은 형태)
//   - 일반 작성자: 메모 그대로
//   - 판정 실패: 이 요청에는 메모를 붙이지 않는다(입력 그대로)

func newAuthorMemoTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// :memory: 는 연결마다 다른 DB 다 — 연결 하나로 묶는다.
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.Exec(`CREATE TABLE g5_member_memo (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		member_id TEXT NOT NULL,
		target_member_id TEXT NOT NULL,
		memo TEXT NOT NULL DEFAULT '',
		memo_detail TEXT NOT NULL DEFAULT '',
		color TEXT NOT NULL DEFAULT 'yellow'
	)`).Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	for _, target := range []string{"left", "normal"} {
		if err := db.Exec(`INSERT INTO g5_member_memo (member_id, target_member_id, memo) VALUES ('me', ?, 'memo')`, target).Error; err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	return db
}

func stubAuthorMemoWithdrawnIDs(t *testing.T, fn func(context.Context, []string) (map[string]bool, error)) {
	t.Helper()
	prev := authorMemoWithdrawnIDs
	authorMemoWithdrawnIDs = fn
	t.Cleanup(func() { authorMemoWithdrawnIDs = prev })
}

func authorMemoItems() []map[string]any {
	return []map[string]any{
		{"id": 1, "author_id": "left"},
		{"id": 2, "author_id": "normal"},
		{"id": 3, "author_id": "nomemo"},
	}
}

func TestEnrichWithAuthorMemo_DropsWithdrawnAuthor(t *testing.T) {
	db := newAuthorMemoTestDB(t)
	stubAuthorMemoWithdrawnIDs(t, func(context.Context, []string) (map[string]bool, error) {
		return map[string]bool{"left": true}, nil
	})

	out := enrichWithAuthorMemo(db, "me", authorMemoItems())
	if len(out) != 3 {
		t.Fatalf("len = %d", len(out))
	}
	for _, i := range []int{0, 2} {
		v, ok := out[i]["author_memo"]
		if !ok {
			t.Fatalf("item %d: author_memo key must be present", i)
		}
		if m, isH := v.(gin.H); isH && m != nil {
			t.Fatalf("item %d: author_memo must be nil, got %#v", i, v)
		}
	}
	m, ok := out[1]["author_memo"].(gin.H)
	if !ok || m["content"] != "memo" {
		t.Fatalf("normal author memo must be kept, got %#v", out[1]["author_memo"])
	}
}

func TestEnrichWithAuthorMemo_LookupFailureAttachesNone(t *testing.T) {
	db := newAuthorMemoTestDB(t)
	stubAuthorMemoWithdrawnIDs(t, func(context.Context, []string) (map[string]bool, error) {
		return nil, errors.New("db down")
	})

	out := enrichWithAuthorMemo(db, "me", authorMemoItems())
	for i, item := range out {
		if _, ok := item["author_memo"]; ok {
			t.Fatalf("item %d: lookup failure must attach no memo, got %#v", i, item["author_memo"])
		}
	}
}
