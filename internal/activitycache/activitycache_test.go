package activitycache

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestTTLIsFiveMinutes(t *testing.T) {
	if TTL != 5*time.Minute {
		t.Fatalf("TTL = %v, want 5m", TTL)
	}
}

func TestKey_MatchesHandlerFormat(t *testing.T) {
	if got := Key("user1", 5); got != "member_activity:user1:5" {
		t.Fatalf("Key = %q", got)
	}
}

func TestKeys_CoversAllLimitsAndWebKey(t *testing.T) {
	keys := Keys("user1")
	if len(keys) != MaxLimit+1 {
		t.Fatalf("len(keys) = %d, want %d", len(keys), MaxLimit+1)
	}
	want := map[string]bool{
		"member_activity:user1:1":  false,
		"member_activity:user1:5":  false,
		"member_activity:user1:20": false,
		"member_activity:user1:50": false,
		"member_activity:user1:l5": false,
	}
	for _, k := range keys {
		if _, ok := want[k]; ok {
			want[k] = true
		}
	}
	for k, found := range want {
		if !found {
			t.Fatalf("missing key %q", k)
		}
	}
	if Keys("") != nil {
		t.Fatal("empty mb_id should give nil")
	}
}

func TestInvalidateMembers_NoRedisIsNoop(_ *testing.T) {
	restore := SetDeleterForTest(nil)
	defer restore()
	InvalidateMembers("user1") // panic 나 블로킹 없이 돌아와야 한다
}

func TestInvalidateMembers_DedupsAndBatchesWithDeadline(t *testing.T) {
	var calls int
	var got []string
	restore := SetDeleterForTest(func(ctx context.Context, keys ...string) error {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("delete must run with a deadline")
		}
		if len(keys) > delBatchKeys {
			t.Fatalf("batch too large: %d", len(keys))
		}
		calls++
		got = append(got, keys...)
		return nil
	})
	defer restore()

	ids := []string{"", "a", "a"}
	for i := 0; i < 30; i++ {
		ids = append(ids, fmt.Sprintf("m%d", i))
	}
	InvalidateMembers(ids...)

	members := 31 // "a" 한 번 + 30명, 빈 값 제외
	if len(got) != members*(MaxLimit+1) {
		t.Fatalf("deleted %d keys, want %d", len(got), members*(MaxLimit+1))
	}
	if calls < 2 {
		t.Fatalf("expected batched deletes, got %d call(s)", calls)
	}
}

func TestInvalidateMembers_ErrorIsSwallowed(_ *testing.T) {
	restore := SetDeleterForTest(func(context.Context, ...string) error {
		return errors.New("redis down")
	})
	defer restore()
	InvalidateMembers("user1") // 에러를 밖으로 내지 않는다
}

func openFeedDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	stmts := []string{
		`CREATE TABLE g5_write_free (
			wr_id INTEGER PRIMARY KEY,
			wr_parent INTEGER NOT NULL DEFAULT 0,
			wr_is_comment INTEGER NOT NULL DEFAULT 0,
			mb_id TEXT
		)`,
		`CREATE TABLE member_activity_feed (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			member_id TEXT NOT NULL,
			write_table TEXT NOT NULL,
			write_id INTEGER NOT NULL,
			parent_write_id INTEGER,
			activity_type INTEGER NOT NULL
		)`,
		`INSERT INTO g5_write_free (wr_id, wr_parent, wr_is_comment, mb_id) VALUES
			(1, 1, 0, 'author'),
			(11, 1, 1, 'c1'),
			(12, 1, 1, 'c2'),
			(13, 1, 1, 'c1')`,
		`INSERT INTO member_activity_feed (member_id, write_table, write_id, parent_write_id, activity_type) VALUES
			('author', 'g5_write_free', 1, NULL, 1),
			('c1', 'g5_write_free', 11, 1, 2),
			('c2', 'g5_write_free', 12, 1, 2),
			('c1', 'g5_write_free', 13, 1, 2),
			('other', 'g5_write_free', 99, 2, 2)`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return db
}

func TestMembersForWrite_PostIncludesCommenters(t *testing.T) {
	db := openFeedDB(t)
	got := MembersForWrite(db, "free", 1)
	sort.Strings(got)
	want := []string{"author", "c1", "c2"}
	if len(got) != len(want) {
		t.Fatalf("members = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("members = %v, want %v", got, want)
		}
	}
}

func TestMembersForWrite_CommentIsAuthorOnly(t *testing.T) {
	db := openFeedDB(t)
	got := MembersForWrite(db, "free", 12)
	if len(got) != 1 || got[0] != "c2" {
		t.Fatalf("members = %v, want [c2]", got)
	}
}

func TestMembersForWrite_RejectsBadBoardSlug(t *testing.T) {
	db := openFeedDB(t)
	if got := MembersForWrite(db, "free`; DROP", 1); got != nil {
		t.Fatalf("bad slug should give nil, got %v", got)
	}
}

func TestInvalidateWrite_DeletesAffectedMembersKeys(t *testing.T) {
	db := openFeedDB(t)
	deleted := map[string]bool{}
	restore := SetDeleterForTest(func(_ context.Context, keys ...string) error {
		for _, k := range keys {
			deleted[k] = true
		}
		return nil
	})
	defer restore()

	InvalidateWrite(db, "free", 1)
	for _, k := range []string{
		"member_activity:author:5",
		"member_activity:c1:5",
		"member_activity:c2:l5",
	} {
		if !deleted[k] {
			t.Fatalf("expected %q to be deleted", k)
		}
	}
	if deleted["member_activity:other:5"] {
		t.Fatal("unrelated member must not be invalidated")
	}
}
