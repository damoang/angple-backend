package v2

import (
	"testing"

	"github.com/damoang/angple-backend/internal/domain"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ⛔ 이 파일이 지키는 계약 (UpdateScope):
//
//   - 이미 같은 범위로 차단돼 있으면 에러가 아니다(MySQL 은 값이 같으면 바뀐 행을 0 으로 준다).
//   - 차단 행이 없으면 에러다.
//   - 다른 범위면 값이 바뀐다.
//
// sqlite 는 값이 같아도 일치한 행 수를 돌려주므로, MySQL 동작은 콜백으로 RowsAffected 를 0 으로 만들어 흉내 낸다.

func newBlockTestRepo(t *testing.T) (*blockRepository, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("sqlite 열기 실패: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql.DB 얻기 실패: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

	ddl := `CREATE TABLE g5_member_block (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		mb_id TEXT NOT NULL,
		blocked_mb_id TEXT NOT NULL,
		block_scope TEXT NOT NULL DEFAULT 'all',
		created_at DATETIME)`
	if err := db.Exec(ddl).Error; err != nil {
		t.Fatalf("테이블 생성 실패: %v", err)
	}
	return &blockRepository{db: db}, db
}

// simulateMySQLUnchangedRows 는 UPDATE 뒤 RowsAffected 를 0 으로 만들어,
// 값이 같을 때 MySQL 이 돌려주는 「바뀐 행 0」 을 재현한다.
func simulateMySQLUnchangedRows(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.Callback().Update().After("gorm:update").Register("test:mysql_unchanged_rows", func(tx *gorm.DB) {
		tx.RowsAffected = 0
	}); err != nil {
		t.Fatalf("콜백 등록 실패: %v", err)
	}
}

func readScope(t *testing.T, db *gorm.DB, mbID, blockedMbID string) string {
	t.Helper()
	var b domain.MemberBlock
	if err := db.Where("mb_id = ? AND blocked_mb_id = ?", mbID, blockedMbID).First(&b).Error; err != nil {
		t.Fatalf("차단 행 읽기 실패: %v", err)
	}
	return b.Scope
}

func TestUpdateScope_SameScopeReturnsNil(t *testing.T) {
	repo, db := newBlockTestRepo(t)
	if _, err := repo.Create("user_a", "user_b", domain.BlockScopeAll); err != nil {
		t.Fatalf("차단 생성 실패: %v", err)
	}
	simulateMySQLUnchangedRows(t, db)

	if err := repo.UpdateScope("user_a", "user_b", domain.BlockScopeAll); err != nil {
		t.Fatalf("같은 범위 재차단은 nil 이어야 한다: %v", err)
	}
	if got := readScope(t, db, "user_a", "user_b"); got != domain.BlockScopeAll {
		t.Fatalf("범위가 바뀌면 안 된다: got %q", got)
	}
}

func TestUpdateScope_MissingRowReturnsError(t *testing.T) {
	repo, _ := newBlockTestRepo(t)
	if err := repo.UpdateScope("user_a", "user_b", domain.BlockScopeAll); err == nil {
		t.Fatal("차단 행이 없으면 에러여야 한다")
	}
}

func TestUpdateScope_DifferentScopeUpdates(t *testing.T) {
	repo, db := newBlockTestRepo(t)
	if _, err := repo.Create("user_a", "user_b", domain.BlockScopeAll); err != nil {
		t.Fatalf("차단 생성 실패: %v", err)
	}
	if err := repo.UpdateScope("user_a", "user_b", domain.BlockScopeMessage); err != nil {
		t.Fatalf("범위 변경 실패: %v", err)
	}
	if got := readScope(t, db, "user_a", "user_b"); got != domain.BlockScopeMessage {
		t.Fatalf("범위가 message 로 바뀌어야 한다: got %q", got)
	}
}
