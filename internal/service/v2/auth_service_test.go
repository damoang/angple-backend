package v2

import (
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
	"github.com/damoang/angple-backend/pkg/jwt"
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

// HIGH-1: 세션 유지 경로(refresh)에도 탈퇴 게이트가 적용되어야 한다.
// 확정(익명화) 계정은 refresh 로 무한 우회할 수 없고(차단), 숙려중이면 grace 상태를 반환한다.
func TestRefreshTokenWithdrawalGate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=private"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec(`CREATE TABLE v2_users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT, email TEXT, password TEXT, nickname TEXT, level INTEGER, status TEXT
	)`).Error; err != nil {
		t.Fatalf("create v2_users: %v", err)
	}
	if err := db.Exec(`CREATE TABLE g5_member (mb_id TEXT PRIMARY KEY, mb_leave_date TEXT)`).Error; err != nil {
		t.Fatalf("create g5_member: %v", err)
	}
	db.Exec(`INSERT INTO v2_users (id, username, nickname, level, status) VALUES (1, 'zoe', '조', 2, 'active')`)

	jwtManager := jwt.NewManager("test-secret", 15, 7)
	svc := NewV2AuthService(v2repo.NewUserRepository(db), jwtManager, nil)
	svc.SetPromotionDeps(db, nil)

	refresh, err := jwtManager.GenerateRefreshToken(strconv.FormatUint(1, 10))
	if err != nil {
		t.Fatalf("gen refresh: %v", err)
	}

	// 확정(40일 경과) → refresh 차단
	db.Exec(`INSERT INTO g5_member (mb_id, mb_leave_date) VALUES ('zoe', ?)`, time.Now().AddDate(0, 0, -40).Format("20060102"))
	if _, err := svc.RefreshToken(refresh); !errors.Is(err, common.ErrAccountWithdrawn) {
		t.Fatalf("confirmed account refresh should be blocked, got %v", err)
	}

	// 숙려중(5일 경과) → grace 상태 반환(차단 아님, 토큰은 발급)
	db.Exec(`UPDATE g5_member SET mb_leave_date = ? WHERE mb_id = 'zoe'`, time.Now().AddDate(0, 0, -5).Format("20060102"))
	resp, err := svc.RefreshToken(refresh)
	if err != nil {
		t.Fatalf("grace refresh should not error, got %v", err)
	}
	if resp.WithdrawalGrace == nil {
		t.Fatal("grace refresh should carry WithdrawalGrace info")
	}
	if resp.AccessToken == "" {
		t.Error("grace refresh should still issue access token (for cancel)")
	}

	// 정상(탈퇴 아님) → 정상 갱신
	db.Exec(`UPDATE g5_member SET mb_leave_date = '' WHERE mb_id = 'zoe'`)
	resp2, err := svc.RefreshToken(refresh)
	if err != nil {
		t.Fatalf("active account refresh should succeed, got %v", err)
	}
	if resp2.WithdrawalGrace != nil {
		t.Error("active account should not carry WithdrawalGrace")
	}
}
