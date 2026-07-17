package v2

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
	"github.com/damoang/angple-backend/pkg/jwt"
	jwtlib "github.com/golang-jwt/jwt/v5"
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

// 웹(damoang.net)이 발급한 refresh 토큰(sub=mb_id, user_id 없음) 수용 경로 검증.
// 서명만으로 통과시키면 로그아웃·로테이션으로 폐기된 토큰까지 살아나므로,
// angple_refresh_tokens 저장소 대조가 반드시 걸려야 한다.
func TestRefreshTokenWebIssued(t *testing.T) {
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
	if err := db.Exec(`CREATE TABLE angple_refresh_tokens (
		token_hash TEXT, mb_id TEXT, expires_at DATETIME, revoked_at DATETIME
	)`).Error; err != nil {
		t.Fatalf("create angple_refresh_tokens: %v", err)
	}
	db.Exec(`INSERT INTO v2_users (id, username, nickname, level, status) VALUES (1, 'google_98050930', '앙', 2, 'active')`)

	jwtManager := jwt.NewManager("test-secret", 15, 7)
	svc := NewV2AuthService(v2repo.NewUserRepository(db), jwtManager, nil)
	svc.SetPromotionDeps(db, nil)

	// 웹 토큰 모사: user_id 없이 sub=mb_id 만. (웹은 SignJWT({sub: mbId}) 로 발급)
	// jti 로 토큰마다 고유화 (같은 초에 발급하면 클레임이 같아 동일 문자열이 된다)
	mintSeq := 0
	mint := func(sub string) string {
		mintSeq++
		tok := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, jwtlib.RegisteredClaims{
			Subject:   sub,
			Issuer:    "angple",
			ID:        strconv.Itoa(mintSeq),
			IssuedAt:  jwtlib.NewNumericDate(time.Now()),
			ExpiresAt: jwtlib.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)),
		})
		signed, mintErr := tok.SignedString([]byte("test-secret"))
		if mintErr != nil {
			t.Fatalf("mint: %v", mintErr)
		}
		return signed
	}
	store := func(tok, mbID string, expires time.Time, revoked bool) {
		sum := sha256.Sum256([]byte(tok))
		h := hex.EncodeToString(sum[:])
		if revoked {
			db.Exec(`INSERT INTO angple_refresh_tokens (token_hash, mb_id, expires_at, revoked_at) VALUES (?,?,?,?)`,
				h, mbID, expires, time.Now())
			return
		}
		db.Exec(`INSERT INTO angple_refresh_tokens (token_hash, mb_id, expires_at, revoked_at) VALUES (?,?,?,NULL)`,
			h, mbID, expires)
	}

	// 1) 저장소에 있고 미폐기·미만료 → 성공
	valid := mint("google_98050930")
	store(valid, "google_98050930", time.Now().Add(24*time.Hour), false)
	resp, err := svc.RefreshToken(valid)
	if err != nil {
		t.Fatalf("valid web token should refresh, got %v", err)
	}
	if resp.User.Username != "google_98050930" || resp.AccessToken == "" {
		t.Fatalf("unexpected refresh response: %+v", resp.User)
	}

	// 2) 로그아웃(폐기)된 토큰 → 거부  ← 저장소 대조가 없으면 여기서 통과해버린다
	revoked := mint("google_98050930")
	store(revoked, "google_98050930", time.Now().Add(24*time.Hour), true)
	if _, err := svc.RefreshToken(revoked); err == nil {
		t.Fatal("revoked web token must be rejected")
	}

	// 3) 만료된 저장소 행 → 거부
	expired := mint("google_98050930")
	store(expired, "google_98050930", time.Now().Add(-1*time.Hour), false)
	if _, err := svc.RefreshToken(expired); err == nil {
		t.Fatal("expired web token must be rejected")
	}

	// 4) 서명은 유효하나 저장소에 없음(app-login 코드·위조 등) → 거부
	unstored := mint("google_98050930")
	if _, err := svc.RefreshToken(unstored); err == nil {
		t.Fatal("web token absent from store must be rejected")
	}

	// 5) sub 와 저장소 mb_id 불일치 → 거부
	mismatch := mint("google_98050930")
	store(mismatch, "other_member", time.Now().Add(24*time.Hour), false)
	if _, err := svc.RefreshToken(mismatch); err == nil {
		t.Fatal("sub/store mb_id mismatch must be rejected")
	}
}
