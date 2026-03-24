package cron

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRunSyncVisibleCommentCountsReconcilesPosts(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	if err := db.Exec(`CREATE TABLE g5_board (bo_table TEXT PRIMARY KEY)`).Error; err != nil {
		t.Fatalf("create g5_board: %v", err)
	}
	if err := db.Exec(`INSERT INTO g5_board (bo_table) VALUES ('free')`).Error; err != nil {
		t.Fatalf("insert g5_board: %v", err)
	}
	if err := db.Exec(`CREATE TABLE g5_write_free (
		wr_id INTEGER PRIMARY KEY,
		wr_parent INTEGER NOT NULL DEFAULT 0,
		wr_is_comment INTEGER NOT NULL DEFAULT 0,
		wr_comment INTEGER NOT NULL DEFAULT 0,
		wr_deleted_at DATETIME NULL
	)`).Error; err != nil {
		t.Fatalf("create g5_write_free: %v", err)
	}
	if err := db.Exec(`
		INSERT INTO g5_write_free (wr_id, wr_parent, wr_is_comment, wr_comment, wr_deleted_at) VALUES
		(1, 1, 0, 99, NULL),
		(2, 2, 0, 3, NULL),
		(11, 1, 1, 0, NULL),
		(12, 1, 1, 0, NULL),
		(13, 1, 1, 0, '2026-03-24 15:00:00')
	`).Error; err != nil {
		t.Fatalf("seed g5_write_free: %v", err)
	}

	result, err := runSyncVisibleCommentCounts(db)
	if err != nil {
		t.Fatalf("runSyncVisibleCommentCounts: %v", err)
	}
	if result.BoardsChecked != 1 {
		t.Fatalf("expected 1 board checked, got %d", result.BoardsChecked)
	}
	if result.BoardsSynced != 1 {
		t.Fatalf("expected 1 board synced, got %d", result.BoardsSynced)
	}

	var count1, count2 int
	if err := db.Table("g5_write_free").Select("wr_comment").Where("wr_id = 1").Scan(&count1).Error; err != nil {
		t.Fatalf("query post 1: %v", err)
	}
	if err := db.Table("g5_write_free").Select("wr_comment").Where("wr_id = 2").Scan(&count2).Error; err != nil {
		t.Fatalf("query post 2: %v", err)
	}
	if count1 != 2 {
		t.Fatalf("expected post 1 wr_comment=2, got %d", count1)
	}
	if count2 != 0 {
		t.Fatalf("expected post 2 wr_comment=0, got %d", count2)
	}
}
