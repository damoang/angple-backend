package handler

import (
	"fmt"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newMemoTargetsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=private"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec(`CREATE TABLE g5_member_memo (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		member_id TEXT NOT NULL,
		target_member_id TEXT NOT NULL,
		memo TEXT NOT NULL DEFAULT '',
		UNIQUE (member_id, target_member_id)
	)`).Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	return db
}

func insertMemo(t *testing.T, db *gorm.DB, member, target, memo string) {
	t.Helper()
	if err := db.Exec(`INSERT INTO g5_member_memo (member_id, target_member_id, memo) VALUES (?, ?, ?)`,
		member, target, memo).Error; err != nil {
		t.Fatalf("insert memo: %v", err)
	}
}

func TestLoadMemoTargets_Empty(t *testing.T) {
	db := newMemoTargetsTestDB(t)
	insertMemo(t, db, "other", "x", "memo")

	got, err := LoadMemoTargets(db, "me", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Targets == nil {
		t.Fatalf("targets must be an empty slice, not nil (JSON [] not null)")
	}
	if len(got.Targets) != 0 || got.Count != 0 || got.Truncated {
		t.Fatalf("expected empty result, got %+v", got)
	}
}

func TestLoadMemoTargets_EmptyMemberID(t *testing.T) {
	db := newMemoTargetsTestDB(t)
	got, err := LoadMemoTargets(db, "", 0)
	if err != nil || got.Count != 0 || got.Targets == nil {
		t.Fatalf("expected empty non-nil result, got %+v err=%v", got, err)
	}
}

func TestLoadMemoTargets_SkipsBlankMemoAndOtherMembers(t *testing.T) {
	db := newMemoTargetsTestDB(t)
	insertMemo(t, db, "me", "a", "hello")
	insertMemo(t, db, "me", "b", "")
	insertMemo(t, db, "me", "c", "world")
	insertMemo(t, db, "other", "d", "memo")

	got, err := LoadMemoTargets(db, "me", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	set := map[string]bool{}
	for _, id := range got.Targets {
		set[id] = true
	}
	if got.Count != 2 || len(set) != 2 || !set["a"] || !set["c"] || got.Truncated {
		t.Fatalf("expected [a c] not truncated, got %+v", got)
	}
}

func TestLoadMemoTargets_ExactlyAtLimitNotTruncated(t *testing.T) {
	db := newMemoTargetsTestDB(t)
	for i := 0; i < 5; i++ {
		insertMemo(t, db, "me", fmt.Sprintf("t%d", i), "m")
	}
	got, err := LoadMemoTargets(db, "me", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Count != 5 || len(got.Targets) != 5 || got.Truncated {
		t.Fatalf("expected 5 targets not truncated, got count=%d truncated=%v", got.Count, got.Truncated)
	}
}

func TestLoadMemoTargets_OverLimitTruncated(t *testing.T) {
	db := newMemoTargetsTestDB(t)
	for i := 0; i < 7; i++ {
		insertMemo(t, db, "me", fmt.Sprintf("t%d", i), "m")
	}
	got, err := LoadMemoTargets(db, "me", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Count != 5 || len(got.Targets) != 5 || !got.Truncated {
		t.Fatalf("expected 5 targets truncated, got count=%d truncated=%v", got.Count, got.Truncated)
	}
}

func TestLoadMemoTargets_DefaultLimit(t *testing.T) {
	if MemoTargetsLimit != 20000 {
		t.Fatalf("MemoTargetsLimit changed: %d", MemoTargetsLimit)
	}
}
