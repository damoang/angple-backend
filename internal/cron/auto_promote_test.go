package cron

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRunAutoPromoteRequiresCertification(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	if err := db.Exec(`CREATE TABLE g5_member (
		mb_id TEXT PRIMARY KEY,
		mb_level INTEGER,
		mb_login_days INTEGER,
		as_exp INTEGER,
		as_level INTEGER,
		mb_certify TEXT,
		mb_datetime DATETIME,
		mb_leave_date TEXT,
		mb_intercept_date TEXT
	)`).Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	if err := db.Exec(`CREATE TABLE g5_member_level_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		mb_id TEXT,
		old_mb_level INTEGER,
		new_mb_level INTEGER,
		reason TEXT,
		snapshot_as_level INTEGER,
		snapshot_as_exp INTEGER,
		snapshot_login_days INTEGER,
		snapshot_mb_certify TEXT,
		member_created_at DATETIME,
		created_at DATETIME
	)`).Error; err != nil {
		t.Fatalf("create history table: %v", err)
	}

	if err := db.Exec(`
		INSERT INTO g5_member (mb_id, mb_level, mb_login_days, as_exp, as_level, mb_certify, mb_datetime, mb_leave_date, mb_intercept_date) VALUES
		('certified_user', 2, 7, 3000, 5, 'simple', '2026-03-01 10:00:00', '', ''),
		('uncertified_user', 2, 30, 9000, 9, '', '2026-03-02 10:00:00', '', '')
	`).Error; err != nil {
		t.Fatalf("insert members: %v", err)
	}

	result, err := runAutoPromote(db, nil)
	if err != nil {
		t.Fatalf("runAutoPromote: %v", err)
	}
	if result.PromotedCount != 1 {
		t.Fatalf("expected 1 promoted member, got %d", result.PromotedCount)
	}

	var certifiedLevel int
	if err := db.Table("g5_member").Select("mb_level").Where("mb_id = ?", "certified_user").Scan(&certifiedLevel).Error; err != nil {
		t.Fatalf("query certified member: %v", err)
	}
	if certifiedLevel != 3 {
		t.Fatalf("expected certified member to be promoted to 3, got %d", certifiedLevel)
	}

	var uncertifiedLevel int
	if err := db.Table("g5_member").Select("mb_level").Where("mb_id = ?", "uncertified_user").Scan(&uncertifiedLevel).Error; err != nil {
		t.Fatalf("query uncertified member: %v", err)
	}
	if uncertifiedLevel != 2 {
		t.Fatalf("expected uncertified member to stay at 2, got %d", uncertifiedLevel)
	}

	var historyCount int64
	if err := db.Table("g5_member_level_history").Where("mb_id = ?", "certified_user").Count(&historyCount).Error; err != nil {
		t.Fatalf("count history: %v", err)
	}
	if historyCount != 1 {
		t.Fatalf("expected 1 history row, got %d", historyCount)
	}
}
