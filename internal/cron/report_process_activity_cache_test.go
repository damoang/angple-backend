package cron

import (
	"context"
	"testing"

	"github.com/damoang/angple-backend/internal/activitycache"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// 신고 묶음 처리 뒤: 피신고자 + 잠금 대상 글의 작성자·댓글 작성자 + 댓글 작성자의
// 활동 캐시가 지워져야 한다.
func TestInvalidateReportGroupActivity_DeletesTargetPostAndCommenters(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	for _, s := range []string{
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
			(1, 1, 0, 'postauthor'),
			(11, 1, 1, 'target'),
			(12, 1, 1, 'bystander')`,
		`INSERT INTO member_activity_feed (member_id, write_table, write_id, parent_write_id, activity_type) VALUES
			('target', 'g5_write_free', 11, 1, 2),
			('bystander', 'g5_write_free', 12, 1, 2)`,
	} {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	deleted := map[string]bool{}
	restore := activitycache.SetDeleterForTest(func(_ context.Context, keys ...string) error {
		for _, k := range keys {
			deleted[k] = true
		}
		return nil
	})
	defer restore()

	// 댓글 신고 1건(부모 글 1). 부모 글은 자동잠금 대상이 될 수 있다.
	invalidateReportGroupActivity(db, "target", []reportedItem{{Table: "free", ID: 11, Parent: 1}})

	for _, k := range []string{
		"member_activity:target:5",
		"member_activity:postauthor:5",
		"member_activity:bystander:5",
		"member_activity:target:l5",
	} {
		if !deleted[k] {
			t.Fatalf("expected %q to be deleted", k)
		}
	}
}
