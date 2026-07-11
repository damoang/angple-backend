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

// setupHelloBoardTest prepares an in-memory DB with the g5_write_hello table
// for the 가입인사 1인 1글 제한 tests.
func setupHelloBoardTest(t *testing.T) (*gorm.DB, *BoardWriteRestrictionService) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	if err := db.AutoMigrate(&v2domain.V2BoardExtendedSettings{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	if err := db.Exec(`
		CREATE TABLE g5_write_hello (
			mb_id TEXT,
			wr_datetime DATETIME,
			wr_is_comment INTEGER,
			wr_deleted_at DATETIME
		)
	`).Error; err != nil {
		t.Fatalf("create g5_write_hello: %v", err)
	}

	svc := NewBoardWriteRestrictionService(db, v2repo.NewBoardExtendedSettingsRepository(db))
	return db, svc
}

func insertHelloRow(t *testing.T, db *gorm.DB, mbID string, isComment int, deletedAt interface{}) {
	t.Helper()
	if err := db.Exec(`
		INSERT INTO g5_write_hello (mb_id, wr_datetime, wr_is_comment, wr_deleted_at)
		VALUES (?, ?, ?, ?)
	`, mbID, time.Now(), isComment, deletedAt).Error; err != nil {
		t.Fatalf("insert hello row: %v", err)
	}
}

func TestHelloBoardDeniesSecondPost(t *testing.T) {
	db, svc := setupHelloBoardTest(t)
	insertHelloRow(t, db, "member-1", 0, nil)

	got, err := svc.Check("hello", "member-1", 2)
	if err != nil {
		t.Fatalf("check restriction: %v", err)
	}

	if got.CanWrite {
		t.Fatalf("expected second hello post to be denied")
	}
	if got.Reason != helloOnePostReason {
		t.Fatalf("unexpected reason: %s", got.Reason)
	}
	if got.TotalLimit != 1 || got.TotalCount != 1 || got.Remaining != 0 {
		t.Fatalf("unexpected counts: limit=%d count=%d remaining=%d", got.TotalLimit, got.TotalCount, got.Remaining)
	}
}

func TestHelloBoardAllowsFirstPost(t *testing.T) {
	_, svc := setupHelloBoardTest(t)

	got, err := svc.Check("hello", "member-1", 2)
	if err != nil {
		t.Fatalf("check restriction: %v", err)
	}

	if !got.CanWrite {
		t.Fatalf("expected first hello post to be allowed, reason: %s", got.Reason)
	}
}

func TestHelloBoardAdminBypassesLimit(t *testing.T) {
	db, svc := setupHelloBoardTest(t)
	insertHelloRow(t, db, "admin-1", 0, nil)

	got, err := svc.Check("hello", "admin-1", 10)
	if err != nil {
		t.Fatalf("check restriction: %v", err)
	}

	if !got.CanWrite {
		t.Fatalf("expected admin (level 10) to bypass hello limit, reason: %s", got.Reason)
	}
}

func TestHelloBoardIgnoresCommentsAndDeletedPosts(t *testing.T) {
	db, svc := setupHelloBoardTest(t)
	// 댓글(wr_is_comment=1)과 삭제된 글(wr_deleted_at 설정)은 카운트에서 제외
	insertHelloRow(t, db, "member-1", 1, nil)
	insertHelloRow(t, db, "member-1", 0, time.Now())

	got, err := svc.Check("hello", "member-1", 2)
	if err != nil {
		t.Fatalf("check restriction: %v", err)
	}

	if !got.CanWrite {
		t.Fatalf("expected comments/deleted posts to be ignored, reason: %s", got.Reason)
	}
}

func TestHelloBoardLimitDoesNotAffectOtherBoards(t *testing.T) {
	db, svc := setupHelloBoardTest(t)
	insertHelloRow(t, db, "member-1", 0, nil)

	if err := db.Exec(`
		CREATE TABLE g5_write_free (
			mb_id TEXT,
			wr_datetime DATETIME,
			wr_is_comment INTEGER,
			wr_deleted_at DATETIME
		)
	`).Error; err != nil {
		t.Fatalf("create g5_write_free: %v", err)
	}

	got, err := svc.Check("free", "member-1", 2)
	if err != nil {
		t.Fatalf("check restriction: %v", err)
	}

	if !got.CanWrite {
		t.Fatalf("expected other boards to be unaffected, reason: %s", got.Reason)
	}
}
