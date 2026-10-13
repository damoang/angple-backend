package middleware

import (
	"context"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// IsWithdrawnMember / WithdrawnMemberIDs 의 계약:
//
//   - 판정 기준은 탈퇴 게이트와 같다: mb_leave_date 가 비어 있지 않고 "0" 이 아니면 탈퇴(숙려 포함).
//   - 게이트(isWithdrawn)와 달리 조회 실패를 error 로 돌려준다 — fail-open 으로 뭉개지 않는다.
//   - DB 미주입이면 판정하지 않는다(false/빈 집합, nil).

func setupWithdrawalTestDB(t *testing.T, withTable bool) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=private"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if withTable {
		if err := db.Exec(`CREATE TABLE g5_member (
			mb_no INTEGER PRIMARY KEY AUTOINCREMENT,
			mb_id TEXT NOT NULL UNIQUE,
			mb_leave_date TEXT NOT NULL DEFAULT ''
		)`).Error; err != nil {
			t.Fatalf("create table: %v", err)
		}
	}
	prevDB, prevRedis := withdrawalCheckDB, withdrawalCheckRedis
	SetWithdrawalCheck(db, nil)
	t.Cleanup(func() { SetWithdrawalCheck(prevDB, prevRedis) })
	return db
}

func insertWithdrawalTestMember(t *testing.T, db *gorm.DB, mbID, leaveDate string) {
	t.Helper()
	if err := db.Exec(`INSERT INTO g5_member (mb_id, mb_leave_date) VALUES (?, ?)`, mbID, leaveDate).Error; err != nil {
		t.Fatalf("insert member: %v", err)
	}
}

func TestIsWithdrawnMember(t *testing.T) {
	db := setupWithdrawalTestDB(t, true)
	insertWithdrawalTestMember(t, db, "left", "20261013")
	insertWithdrawalTestMember(t, db, "normal", "")
	insertWithdrawalTestMember(t, db, "zero", "0")

	tests := []struct {
		mbID string
		want bool
	}{
		{"left", true},
		{"normal", false},
		{"zero", false},
		{"missing", false},
	}
	for _, tt := range tests {
		got, err := IsWithdrawnMember(context.Background(), tt.mbID)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tt.mbID, err)
		}
		if got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.mbID, got, tt.want)
		}
	}
}

func TestIsWithdrawnMember_GraceCancelVisibleAgain(t *testing.T) {
	db := setupWithdrawalTestDB(t, true)
	insertWithdrawalTestMember(t, db, "grace", "20261013")

	if got, err := IsWithdrawnMember(context.Background(), "grace"); err != nil || !got {
		t.Fatalf("during grace: got %v err=%v, want true", got, err)
	}
	// 숙려 중 취소 = mb_leave_date 를 비운다.
	if err := db.Exec(`UPDATE g5_member SET mb_leave_date = '' WHERE mb_id = 'grace'`).Error; err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if got, err := IsWithdrawnMember(context.Background(), "grace"); err != nil || got {
		t.Fatalf("after cancel: got %v err=%v, want false", got, err)
	}
}

func TestIsWithdrawnMember_LookupFailureReturnsError(t *testing.T) {
	setupWithdrawalTestDB(t, false) // g5_member 없음 → 조회 실패

	got, err := IsWithdrawnMember(context.Background(), "anyone")
	if err == nil {
		t.Fatalf("expected error on lookup failure, got withdrawn=%v", got)
	}
	// 게이트(isWithdrawn)는 기존대로 fail-open 이어야 한다.
	if isWithdrawn(context.Background(), "anyone") {
		t.Fatalf("gate must stay fail-open on lookup failure")
	}
}

func TestIsWithdrawnMember_NoDBSkips(t *testing.T) {
	prevDB, prevRedis := withdrawalCheckDB, withdrawalCheckRedis
	SetWithdrawalCheck(nil, nil)
	t.Cleanup(func() { SetWithdrawalCheck(prevDB, prevRedis) })

	got, err := IsWithdrawnMember(context.Background(), "anyone")
	if err != nil || got {
		t.Fatalf("nil DB: got %v err=%v, want false nil", got, err)
	}
	ids, err := WithdrawnMemberIDs(context.Background(), []string{"a"})
	if err != nil || len(ids) != 0 {
		t.Fatalf("nil DB: got %v err=%v, want empty nil", ids, err)
	}
}

func TestWithdrawnMemberIDs(t *testing.T) {
	db := setupWithdrawalTestDB(t, true)
	insertWithdrawalTestMember(t, db, "left1", "20261001")
	insertWithdrawalTestMember(t, db, "left2", "20261013")
	insertWithdrawalTestMember(t, db, "normal", "")
	insertWithdrawalTestMember(t, db, "zero", "0")

	got, err := WithdrawnMemberIDs(context.Background(), []string{"left1", "normal", "zero", "missing", "left2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 || !got["left1"] || !got["left2"] {
		t.Fatalf("got %v, want {left1, left2}", got)
	}

	empty, err := WithdrawnMemberIDs(context.Background(), nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty ids: got %v err=%v", empty, err)
	}
}

func TestWithdrawnMemberIDs_LookupFailureReturnsError(t *testing.T) {
	setupWithdrawalTestDB(t, false)

	if _, err := WithdrawnMemberIDs(context.Background(), []string{"a", "b"}); err == nil {
		t.Fatalf("expected error on lookup failure")
	}
}

func TestWithdrawnLeaveDateSQLMatchesGoRule(t *testing.T) {
	if got := WithdrawnLeaveDateSQL("gm.mb_leave_date"); got != "(gm.mb_leave_date <> '' AND gm.mb_leave_date <> '0')" {
		t.Fatalf("unexpected SQL: %s", got)
	}
	for v, want := range map[string]bool{"": false, "0": false, "20261013": true} {
		if isWithdrawnLeaveDate(v) != want {
			t.Errorf("isWithdrawnLeaveDate(%q) != %v", v, want)
		}
	}
}
