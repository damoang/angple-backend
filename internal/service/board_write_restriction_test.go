package service

import (
	"testing"
	"time"

	v2domain "github.com/damoang/angple-backend/internal/domain/v2"
	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestBoardWriteRestrictionShadowPolicyDoesNotOverrideBaseDecision(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	if err := db.AutoMigrate(&v2domain.V2BoardExtendedSettings{}, &v2domain.V2AdvertiserBoardPolicy{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	if err := db.Exec(`
		CREATE TABLE promotions (
			member_id TEXT,
			is_active BOOLEAN,
			start_date TEXT,
			end_date TEXT,
			permission_group TEXT
		)
	`).Error; err != nil {
		t.Fatalf("create promotions: %v", err)
	}

	if err := db.Exec(`
		CREATE TABLE g5_write_promotion (
			mb_id TEXT,
			wr_datetime DATETIME,
			wr_is_comment INTEGER,
			wr_deleted_at DATETIME
		)
	`).Error; err != nil {
		t.Fatalf("create g5_write_promotion: %v", err)
	}

	settingsRepo := v2repo.NewBoardExtendedSettingsRepository(db)
	if err := settingsRepo.Upsert(&v2domain.V2BoardExtendedSettings{
		BoardID:  "promotion",
		Settings: `{"writing":{"restrictedUsers":true,"allowedMembersOne":"legacy-only"}}`,
	}); err != nil {
		t.Fatalf("seed settings: %v", err)
	}

	policyRepo := v2repo.NewAdvertiserBoardPolicyRepository(db)
	if err := policyRepo.Upsert(&v2domain.V2AdvertiserBoardPolicy{
		BoardID:                    "promotion",
		Enabled:                    true,
		Mode:                       AdvertiserPolicyModeShadow,
		AllowActiveAdvertiserWrite: true,
	}); err != nil {
		t.Fatalf("seed policy: %v", err)
	}

	today := time.Now().Format("2006-01-02")
	if err := db.Exec(`
		INSERT INTO promotions (member_id, is_active, start_date, end_date, permission_group)
		VALUES (?, 1, ?, ?, 'one')
	`, "ad-1", today, today).Error; err != nil {
		t.Fatalf("seed promotion: %v", err)
	}

	svc := NewBoardWriteRestrictionService(db, settingsRepo)
	svc.SetAdvertiserPolicyService(NewAdvertiserBoardPolicyService(db, policyRepo))

	got, err := svc.Check("promotion", "ad-1", 5)
	if err != nil {
		t.Fatalf("check restriction: %v", err)
	}

	if got.CanWrite {
		t.Fatalf("expected base restriction to remain denied in shadow mode")
	}
	if got.Reason != "허용된 회원만 글을 작성할 수 있습니다." {
		t.Fatalf("unexpected reason: %s", got.Reason)
	}
}
