package v2

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestIsEligibleForAutoPromotion(t *testing.T) {
	tests := []struct {
		name      string
		level     int
		loginDays int
		exp       int
		certify   string
		want      bool
	}{
		{
			name:      "eligible certified member",
			level:     2,
			loginDays: 7,
			exp:       3000,
			certify:   "simple",
			want:      true,
		},
		{
			name:      "rejects uncertified member",
			level:     2,
			loginDays: 10,
			exp:       12000,
			certify:   "",
			want:      false,
		},
		{
			name:      "rejects wrong level",
			level:     3,
			loginDays: 10,
			exp:       12000,
			certify:   "simple",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isEligibleForAutoPromotion(tt.level, tt.loginDays, tt.exp, tt.certify)
			if got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

func TestCheckAndPromoteWritesMemberLevelHistory(t *testing.T) {
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
		mb_datetime DATETIME
	)`).Error; err != nil {
		t.Fatalf("create member table: %v", err)
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
		INSERT INTO g5_member (mb_id, mb_level, mb_login_days, as_exp, as_level, mb_certify, mb_datetime)
		VALUES ('certified_user', 2, 7, 3000, 4, 'simple', '2026-03-01 10:00:00')
	`).Error; err != nil {
		t.Fatalf("insert member: %v", err)
	}

	svc := &V2AuthService{db: db}

	promoted, newLevel := svc.checkAndPromote("certified_user")
	if !promoted {
		t.Fatalf("expected promotion to succeed")
	}
	if newLevel != 3 {
		t.Fatalf("expected new level 3, got %d", newLevel)
	}

	var history struct {
		OldMbLevel        int    `gorm:"column:old_mb_level"`
		NewMbLevel        int    `gorm:"column:new_mb_level"`
		Reason            string `gorm:"column:reason"`
		SnapshotAsLevel   int    `gorm:"column:snapshot_as_level"`
		SnapshotAsExp     int    `gorm:"column:snapshot_as_exp"`
		SnapshotLoginDays int    `gorm:"column:snapshot_login_days"`
	}
	if err := db.Table("g5_member_level_history").Where("mb_id = ?", "certified_user").Take(&history).Error; err != nil {
		t.Fatalf("query history: %v", err)
	}

	if history.OldMbLevel != 2 || history.NewMbLevel != 3 {
		t.Fatalf("unexpected level history: %+v", history)
	}
	if history.Reason != "auto_promote_login_api" {
		t.Fatalf("unexpected reason: %s", history.Reason)
	}
	if history.SnapshotAsLevel != 4 || history.SnapshotAsExp != 3000 || history.SnapshotLoginDays != 7 {
		t.Fatalf("unexpected snapshot history: %+v", history)
	}
}
