package service

import (
	"context"
	"database/sql"
	"testing"

	"github.com/damoang/angple-backend/internal/activitycache"
	sqlite3 "github.com/mattn/go-sqlite3"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// 피드 동기화 쿼리가 쓰는 MySQL DATE_FORMAT 을 sqlite 에 흉내 낸다(값을 그대로 돌려준다).
var activitySyncSqliteDriverRegistered = registerActivitySyncSqliteDriver()

func registerActivitySyncSqliteDriver() bool {
	sql.Register("sqlite3_activity_sync", &sqlite3.SQLiteDriver{
		ConnectHook: func(conn *sqlite3.SQLiteConn) error {
			return conn.RegisterFunc("DATE_FORMAT", func(v, _ string) string { return v }, true)
		},
	})
	return true
}

func openActivitySyncDB(t *testing.T) *gorm.DB {
	t.Helper()
	_ = activitySyncSqliteDriverRegistered
	db, err := gorm.Open(sqlite.Dialector{DriverName: "sqlite3_activity_sync", DSN: "file::memory:?cache=private"}, &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

	for _, s := range []string{
		`CREATE TABLE g5_board (bo_table TEXT PRIMARY KEY, bo_use_search INTEGER NOT NULL DEFAULT 1)`,
		`INSERT INTO g5_board (bo_table, bo_use_search) VALUES ('free', 1)`,
		`CREATE TABLE g5_write_free (
			wr_id INTEGER PRIMARY KEY,
			wr_parent INTEGER NOT NULL DEFAULT 0,
			wr_is_comment INTEGER NOT NULL DEFAULT 0,
			mb_id TEXT,
			wr_name TEXT,
			wr_subject TEXT,
			wr_content TEXT,
			wr_option TEXT,
			wr_hit INTEGER DEFAULT 0,
			wr_good INTEGER DEFAULT 0,
			wr_nogood INTEGER DEFAULT 0,
			wr_comment INTEGER DEFAULT 0,
			wr_file INTEGER DEFAULT 0,
			wr_deleted_at DATETIME NULL,
			wr_datetime DATETIME,
			wr_last TEXT
		)`,
		`INSERT INTO g5_write_free (wr_id, wr_parent, wr_is_comment, mb_id, wr_name, wr_subject, wr_content, wr_option, wr_datetime, wr_last) VALUES
			(1, 1, 0, 'author', '작성자', '제목', '본문', '', '2026-10-16 10:00:00', ''),
			(11, 1, 1, 'commenter', '댓글러', '', '댓글', '', '2026-10-16 10:05:00', '')`,
		`CREATE TABLE member_activity_feed (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			member_id TEXT NOT NULL,
			board_id TEXT NOT NULL,
			write_table TEXT NOT NULL,
			write_id INTEGER NOT NULL,
			parent_write_id INTEGER,
			activity_type INTEGER NOT NULL,
			is_public INTEGER NOT NULL DEFAULT 1,
			is_deleted INTEGER NOT NULL DEFAULT 0,
			title TEXT,
			content_preview TEXT,
			content_kind TEXT,
			parent_title TEXT,
			author_name TEXT,
			wr_option TEXT,
			view_count INTEGER NOT NULL DEFAULT 0,
			like_count INTEGER NOT NULL DEFAULT 0,
			dislike_count INTEGER NOT NULL DEFAULT 0,
			comment_count INTEGER NOT NULL DEFAULT 0,
			has_file INTEGER NOT NULL DEFAULT 0,
			source_created_at DATETIME NOT NULL,
			source_updated_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE (write_table, write_id, activity_type)
		)`,
		`CREATE TABLE member_activity_stats (
			member_id TEXT NOT NULL,
			board_id TEXT NOT NULL DEFAULT '',
			post_count INTEGER NOT NULL DEFAULT 0,
			comment_count INTEGER NOT NULL DEFAULT 0,
			public_post_count INTEGER NOT NULL DEFAULT 0,
			public_comment_count INTEGER NOT NULL DEFAULT 0
		)`,
	} {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return db
}

// deleteRecorder 는 삭제가 불린 시점에 피드 행이 이미 있었는지 함께 기록한다.
type deleteRecorder struct {
	db          *gorm.DB
	writeID     int
	actType     int
	deleted     map[string]bool
	feedAtCalls []int64
}

func (r *deleteRecorder) del(_ context.Context, keys ...string) error {
	for _, k := range keys {
		r.deleted[k] = true
	}
	var n int64
	r.db.Table("member_activity_feed").
		Where("write_table = 'g5_write_free' AND write_id = ? AND activity_type = ?", r.writeID, r.actType).
		Count(&n)
	r.feedAtCalls = append(r.feedAtCalls, n)
	return nil
}

func TestSyncLegacyComment_InvalidatesAfterFeedUpsert(t *testing.T) {
	db := openActivitySyncDB(t)
	rec := &deleteRecorder{db: db, writeID: 11, actType: 2, deleted: map[string]bool{}}
	restore := activitycache.SetDeleterForTest(rec.del)
	defer restore()

	if err := NewMemberActivitySyncService(db).SyncLegacyComment("free", 11); err != nil {
		t.Fatalf("SyncLegacyComment: %v", err)
	}
	if !rec.deleted["member_activity:commenter:5"] || !rec.deleted["member_activity:commenter:l5"] {
		t.Fatalf("comment author cache not invalidated: %v", rec.deleted)
	}
	if len(rec.feedAtCalls) == 0 {
		t.Fatal("invalidation was not called")
	}
	for _, n := range rec.feedAtCalls {
		if n != 1 {
			t.Fatalf("invalidation ran before the feed upsert (feed rows at call = %d)", n)
		}
	}
}

func TestSyncLegacyPost_InvalidatesAuthorAndCommentersAfterFeedUpsert(t *testing.T) {
	db := openActivitySyncDB(t)
	// 댓글이 먼저 피드에 있어야 글 동기화가 댓글 작성자를 찾는다.
	restoreNoop := activitycache.SetDeleterForTest(nil)
	if err := NewMemberActivitySyncService(db).SyncLegacyComment("free", 11); err != nil {
		t.Fatalf("seed comment feed: %v", err)
	}
	restoreNoop()

	rec := &deleteRecorder{db: db, writeID: 1, actType: 1, deleted: map[string]bool{}}
	restore := activitycache.SetDeleterForTest(rec.del)
	defer restore()

	if err := NewMemberActivitySyncService(db).SyncLegacyPost("free", 1); err != nil {
		t.Fatalf("SyncLegacyPost: %v", err)
	}
	for _, k := range []string{"member_activity:author:5", "member_activity:commenter:5"} {
		if !rec.deleted[k] {
			t.Fatalf("expected %q to be deleted, got %v", k, rec.deleted)
		}
	}
	for _, n := range rec.feedAtCalls {
		if n != 1 {
			t.Fatalf("invalidation ran before the feed upsert (feed rows at call = %d)", n)
		}
	}
}
