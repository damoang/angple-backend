package gnuboard

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type notiPrefRow struct {
	NotiComment int `gorm:"column:noti_comment"`
	NotiFollow  int `gorm:"column:noti_follow"`
	NotiKeyword int `gorm:"column:noti_keyword"`
}

func readNotiPrefRow(t *testing.T, db *gorm.DB, mbID string) notiPrefRow {
	t.Helper()
	var row notiPrefRow
	if err := db.Raw("SELECT noti_comment, noti_follow, noti_keyword FROM g5_noti_preference WHERE mb_id = ?", mbID).
		Scan(&row).Error; err != nil {
		t.Fatalf("select: %v", err)
	}
	return row
}

// 행이 없는 회원이 처음 저장할 때 false(끔)가 컬럼 기본값(1)으로 바뀌면 안 된다.
func TestNotiPreferenceUpsertPersistsFalseOnFirstCreate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:noti_pref_upsert?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.Exec(`CREATE TABLE g5_noti_preference (
		mb_id TEXT PRIMARY KEY,
		noti_comment INTEGER NOT NULL DEFAULT 1,
		noti_reply INTEGER NOT NULL DEFAULT 1,
		noti_mention INTEGER NOT NULL DEFAULT 1,
		noti_like INTEGER NOT NULL DEFAULT 1,
		noti_follow INTEGER NOT NULL DEFAULT 1,
		noti_board_subscribe INTEGER NOT NULL DEFAULT 1,
		noti_keyword INTEGER NOT NULL DEFAULT 1,
		like_threshold INTEGER NOT NULL DEFAULT 1,
		updated_at DATETIME
	)`).Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	repo := NewNotiPreferenceRepository(db)

	pref, err := repo.Get("u1")
	if err != nil {
		t.Fatalf("get defaults: %v", err)
	}
	pref.NotiKeyword = false
	pref.NotiFollow = false
	if err := repo.Upsert(pref); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	got := readNotiPrefRow(t, db, "u1")
	if got.NotiKeyword != 0 || got.NotiFollow != 0 || got.NotiComment != 1 {
		t.Fatalf("첫 저장에서 끈 값이 그대로 저장돼야 한다: %+v", got)
	}

	// 기존 행 갱신 경로도 같은 값을 쓴다.
	pref.NotiKeyword = true
	if err := repo.Upsert(pref); err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	got = readNotiPrefRow(t, db, "u1")
	if got.NotiKeyword != 1 || got.NotiFollow != 0 {
		t.Fatalf("갱신 결과가 다르다: %+v", got)
	}
}
