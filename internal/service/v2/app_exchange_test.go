package v2

import (
	"errors"
	"testing"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
	pkgjwt "github.com/damoang/angple-backend/pkg/jwt"
	golangjwt "github.com/golang-jwt/jwt/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const appExchangeTestSecret = "test-secret"

// mintAppLoginCode simulates the web frontend minting a short-lived app-login
// code (HS256, shared secret, aud="app-login", sub=mb_id).
func mintAppLoginCode(t *testing.T, sub, nickname, email string, level int, aud string, ttl time.Duration) string {
	t.Helper()
	claims := &pkgjwt.Claims{
		Nickname: nickname,
		Level:    level,
		Email:    email,
		RegisteredClaims: golangjwt.RegisteredClaims{
			Subject:   sub,
			Audience:  golangjwt.ClaimStrings{aud},
			ExpiresAt: golangjwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  golangjwt.NewNumericDate(time.Now()),
		},
	}
	token := golangjwt.NewWithClaims(golangjwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(appExchangeTestSecret))
	if err != nil {
		t.Fatalf("sign app-login code: %v", err)
	}
	return signed
}

func newAppExchangeTestService(t *testing.T) (*V2AuthService, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=private"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec(`CREATE TABLE v2_users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT, email TEXT, password TEXT, nickname TEXT, level INTEGER, status TEXT,
		point INTEGER DEFAULT 0, exp INTEGER DEFAULT 0,
		nariya_level INTEGER DEFAULT 1, nariya_max INTEGER DEFAULT 1000,
		avatar_url TEXT, bio TEXT,
		created_at DATETIME, updated_at DATETIME
	)`).Error; err != nil {
		t.Fatalf("create v2_users: %v", err)
	}
	jwtManager := pkgjwt.NewManager(appExchangeTestSecret, 900, 604800)
	svc := NewV2AuthService(v2repo.NewUserRepository(db), jwtManager, nil)
	return svc, db
}

func TestAppExchangeLoginExistingUser(t *testing.T) {
	svc, db := newAppExchangeTestService(t)
	db.Exec(`INSERT INTO v2_users (id, username, nickname, level, status) VALUES (7, 'sundo', '순도', 3, 'active')`)

	code := mintAppLoginCode(t, "sundo", "순도", "s@example.com", 3, AppLoginAudience, time.Minute)
	resp, err := svc.AppExchangeLogin(code)
	if err != nil {
		t.Fatalf("exchange should succeed, got %v", err)
	}
	if resp.User.ID != 7 || resp.User.Username != "sundo" {
		t.Fatalf("unexpected user: %+v", resp.User)
	}
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Fatal("token pair must be issued")
	}
}

func TestAppExchangeLoginAutoProvision(t *testing.T) {
	svc, db := newAppExchangeTestService(t)

	code := mintAppLoginCode(t, "newbie", "새싹", "n@example.com", 2, AppLoginAudience, time.Minute)
	resp, err := svc.AppExchangeLogin(code)
	if err != nil {
		t.Fatalf("exchange should auto-provision, got %v", err)
	}
	if resp.User.Username != "newbie" || resp.User.Nickname != "새싹" {
		t.Fatalf("unexpected provisioned user: %+v", resp.User)
	}

	var count int64
	db.Table("v2_users").Where("username = ?", "newbie").Count(&count)
	if count != 1 {
		t.Fatalf("expected 1 provisioned row, got %d", count)
	}
}

func TestAppExchangeLoginRejectsWrongAudience(t *testing.T) {
	svc, db := newAppExchangeTestService(t)
	db.Exec(`INSERT INTO v2_users (id, username, nickname, level, status) VALUES (1, 'sundo', '순도', 3, 'active')`)

	// aud 없는 일반 refresh 토큰 → 거부
	noAud := mintAppLoginCode(t, "sundo", "", "", 0, "other-aud", time.Minute)
	if _, err := svc.AppExchangeLogin(noAud); !errors.Is(err, common.ErrUnauthorized) {
		t.Fatalf("wrong audience must be rejected, got %v", err)
	}
}

func TestAppExchangeLoginRejectsExpired(t *testing.T) {
	svc, _ := newAppExchangeTestService(t)
	expired := mintAppLoginCode(t, "sundo", "", "", 0, AppLoginAudience, -time.Minute)
	if _, err := svc.AppExchangeLogin(expired); !errors.Is(err, common.ErrUnauthorized) {
		t.Fatalf("expired code must be rejected, got %v", err)
	}
}

func TestAppExchangeLoginRejectsBanned(t *testing.T) {
	svc, db := newAppExchangeTestService(t)
	db.Exec(`INSERT INTO v2_users (id, username, nickname, level, status) VALUES (1, 'baddie', '벤', 1, 'banned')`)

	code := mintAppLoginCode(t, "baddie", "벤", "", 1, AppLoginAudience, time.Minute)
	if _, err := svc.AppExchangeLogin(code); !errors.Is(err, common.ErrUnauthorized) {
		t.Fatalf("banned user must be rejected, got %v", err)
	}
}
