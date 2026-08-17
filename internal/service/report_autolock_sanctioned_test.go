package service

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// newSanctionTestDB 는 g5_na_singo 최소 스키마를 가진 인메모리 DB 를 만든다.
func newSanctionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("sqlite 열기 실패: %v", err)
	}
	if err := db.Exec(`CREATE TABLE g5_na_singo (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		sg_table TEXT, sg_id INTEGER, mb_id TEXT,
		processed INTEGER DEFAULT 0, admin_approved INTEGER DEFAULT 0)`).Error; err != nil {
		t.Fatalf("테이블 생성 실패: %v", err)
	}
	return db
}

func insertSingo(t *testing.T, db *gorm.DB, table string, sgID, processed, approved int) {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO g5_na_singo (sg_table, sg_id, processed, admin_approved) VALUES (?,?,?,?)`,
		table, sgID, processed, approved).Error; err != nil {
		t.Fatalf("신고행 삽입 실패: %v", err)
	}
}

// TestContentSanctioned 는 자동잠금이 **재잠금을 건너뛸 조건**을 잠근다.
//
// ⛔ 새 잠금은 wr_7 만 바꾸는 게 아니라 작성자 냉각(ApplyReportFreeze)과
// 진실의 방 참조글까지 만든다. 처분이 끝난 글에 또 걸리면 중복 처분이다.
func TestContentSanctioned(t *testing.T) {
	db := newSanctionTestDB(t)

	// 100: 신고만 있고 미처리 → 잠글 수 있어야 한다
	insertSingo(t, db, "free", 100, 0, 0)
	// 200: 각하(처리했으나 미승인) → ⭐ 잠글 수 있어야 한다
	insertSingo(t, db, "free", 200, 1, 0)
	// 300: 제재확정 → ⛔ 잠그지 않는다
	insertSingo(t, db, "free", 300, 1, 1)
	// 400: 같은 글에 각하 1행 + 제재확정 1행 → ⛔ 잠그지 않는다
	insertSingo(t, db, "free", 400, 1, 0)
	insertSingo(t, db, "free", 400, 1, 1)

	tests := []struct {
		name  string
		table string
		sgID  int
		want  bool
	}{
		{"미처리 — 잠글 수 있다", "free", 100, false},
		{"각하 — 잠글 수 있다", "free", 200, false},
		{"제재확정 — 재잠금 금지", "free", 300, true},
		{"각하+제재확정 혼재 — 재잠금 금지", "free", 400, true},
		{"신고 이력 없음", "free", 999, false},
		{"다른 게시판의 같은 번호 — 섞이면 안 된다", "qa", 300, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := contentSanctioned(db, tt.table, tt.sgID); got != tt.want {
				t.Errorf("contentSanctioned(%s, %d) = %v, want %v", tt.table, tt.sgID, got, tt.want)
			}
		})
	}
}

// ⛔ 조회가 실패하면 true 여야 한다(fail-closed).
// 모르는 상태에서 작성자에게 냉각 제한을 거는 쪽이, 잠금이 한 번 늦는 쪽보다 나쁘다.
func TestContentSanctioned_FailClosed(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("sqlite 열기 실패: %v", err)
	}
	// g5_na_singo 를 만들지 않는다 → 조회가 실패한다.
	if got := contentSanctioned(db, "free", 1); !got {
		t.Error("조회 실패 시 true(재잠금 보류)여야 한다 — false 면 작성자에게 제한이 또 걸린다")
	}
}
