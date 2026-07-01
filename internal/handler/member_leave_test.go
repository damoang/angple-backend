package handler

import (
	"errors"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newLeaveTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=private"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec(`CREATE TABLE g5_member (
		mb_no INTEGER PRIMARY KEY AUTOINCREMENT,
		mb_id TEXT,
		mb_nick TEXT,
		mb_nick_date TEXT,
		mb_leave_date TEXT,
		mb_leave_reason TEXT,
		mb_intercept_date TEXT,
		mb_dupinfo TEXT,
		mb_ip TEXT
	)`).Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	return db
}

func leaveDateAgo(days int) string {
	return time.Now().AddDate(0, 0, -days).Format("20060102")
}

func getMember(t *testing.T, db *gorm.DB, mbID string) (leave, reason, intercept, nick string) {
	t.Helper()
	var row struct {
		MbLeaveDate     string `gorm:"column:mb_leave_date"`
		MbLeaveReason   string `gorm:"column:mb_leave_reason"`
		MbInterceptDate string `gorm:"column:mb_intercept_date"`
		MbNick          string `gorm:"column:mb_nick"`
	}
	if err := db.Table("g5_member").Where("mb_id = ?", mbID).Take(&row).Error; err != nil {
		t.Fatalf("get member %s: %v", mbID, err)
	}
	return row.MbLeaveDate, row.MbLeaveReason, row.MbInterceptDate, row.MbNick
}

// 탈퇴 신청 → mb_leave_date/reason 세팅, intercept 불변.
func TestApplySelfLeave(t *testing.T) {
	db := newLeaveTestDB(t)
	db.Exec(`INSERT INTO g5_member (mb_id, mb_nick, mb_leave_date, mb_leave_reason, mb_intercept_date)
		VALUES ('alice', '앨리스', '', '', '')`)

	if _, err := applySelfLeave(db, "alice", "충동탈퇴", time.Now()); err != nil {
		t.Fatalf("applySelfLeave: %v", err)
	}
	leave, reason, intercept, _ := getMember(t, db, "alice")
	if leave == "" {
		t.Error("mb_leave_date should be set")
	}
	if reason != "충동탈퇴" {
		t.Errorf("reason = %q, want 충동탈퇴", reason)
	}
	if intercept != "" {
		t.Errorf("mb_intercept_date must remain untouched, got %q", intercept)
	}
}

// 이미 숙려중이면 신청일(clock)을 재설정하지 않는다.
func TestApplySelfLeaveDoesNotResetClock(t *testing.T) {
	db := newLeaveTestDB(t)
	old := leaveDateAgo(10)
	db.Exec(`INSERT INTO g5_member (mb_id, mb_nick, mb_leave_date, mb_leave_reason) VALUES ('bob', '밥', ?, 'self')`, old)

	if _, err := applySelfLeave(db, "bob", "again", time.Now()); err != nil {
		t.Fatalf("applySelfLeave: %v", err)
	}
	leave, _, _, _ := getMember(t, db, "bob")
	if leave != old {
		t.Errorf("leave_date should stay %q (no clock reset), got %q", old, leave)
	}
}

// 가드 #1: 취소는 mb_leave_date 만 지우고 mb_intercept_date 는 절대 건드리지 않는다(제재 세탁 방지).
func TestCancelSelfLeavePreservesIntercept(t *testing.T) {
	db := newLeaveTestDB(t)
	db.Exec(`INSERT INTO g5_member (mb_id, mb_nick, mb_leave_date, mb_leave_reason, mb_intercept_date)
		VALUES ('carol', '캐롤', ?, 'self', '20260615')`, leaveDateAgo(3))

	if err := cancelSelfLeave(db, "carol", time.Now()); err != nil {
		t.Fatalf("cancelSelfLeave: %v", err)
	}
	leave, reason, intercept, _ := getMember(t, db, "carol")
	if leave != "" || reason != "" {
		t.Errorf("leave_date/reason should be cleared, got leave=%q reason=%q", leave, reason)
	}
	if intercept != "20260615" {
		t.Errorf("mb_intercept_date must be preserved (제재 유지), got %q", intercept)
	}
}

// 숙려기간(30일) 경과 시 취소 불가.
func TestCancelSelfLeaveGraceElapsed(t *testing.T) {
	db := newLeaveTestDB(t)
	db.Exec(`INSERT INTO g5_member (mb_id, mb_nick, mb_leave_date, mb_leave_reason) VALUES ('dave', '데이브', ?, 'self')`, leaveDateAgo(40))

	err := cancelSelfLeave(db, "dave", time.Now())
	if !errors.Is(err, errGraceElapsed) {
		t.Fatalf("expected errGraceElapsed, got %v", err)
	}
	// 실패 시 leave_date 는 그대로 유지되어야 함.
	leave, _, _, _ := getMember(t, db, "dave")
	if leave == "" {
		t.Error("leave_date should remain set when cancel refused")
	}
}

// 이미 익명화 확정된 계정은 (숙려중이라도) 복구 불가.
func TestCancelSelfLeaveAlreadyAnonymized(t *testing.T) {
	db := newLeaveTestDB(t)
	db.Exec(`INSERT INTO g5_member (mb_id, mb_nick, mb_leave_date) VALUES ('erin', '탈퇴회원_9', ?)`, leaveDateAgo(3))

	if err := cancelSelfLeave(db, "erin", time.Now()); !errors.Is(err, errAlreadyAnonymize) {
		t.Fatalf("expected errAlreadyAnonymize, got %v", err)
	}
}

// 탈퇴 신청 상태가 아니면 취소 불가.
func TestCancelSelfLeaveNotWithdrawing(t *testing.T) {
	db := newLeaveTestDB(t)
	db.Exec(`INSERT INTO g5_member (mb_id, mb_nick, mb_leave_date) VALUES ('frank', '프랭크', '')`)

	if err := cancelSelfLeave(db, "frank", time.Now()); !errors.Is(err, errNotWithdrawing) {
		t.Fatalf("expected errNotWithdrawing, got %v", err)
	}
}

// HIGH-2: 숙려중 사용자는 grace access_token(Bearer)만 보유 → JWT subject 가 v2_users.id(숫자).
// resolveMbID 는 이 숫자 subject 를 g5_member.mb_id(username)로 역해석해야 한다.
func TestResolveMbID(t *testing.T) {
	db := newLeaveTestDB(t)
	db.Exec(`CREATE TABLE v2_users (id INTEGER PRIMARY KEY, username TEXT)`)
	db.Exec(`INSERT INTO v2_users (id, username) VALUES (42, 'grace_user')`)

	// username 클레임이 있으면 그대로 사용
	if got := resolveMbID(db, "grace_user", "42"); got != "grace_user" {
		t.Errorf("username claim should win, got %q", got)
	}
	// username 클레임 없이 숫자 subject 만 → v2_users 역해석
	if got := resolveMbID(db, "", "42"); got != "grace_user" {
		t.Errorf("numeric subject should resolve to username, got %q", got)
	}
	// 비숫자 subject(내부 경로)는 이미 mb_id
	if got := resolveMbID(db, "", "alice"); got != "alice" {
		t.Errorf("non-numeric subject should pass through, got %q", got)
	}
	// 미인증
	if got := resolveMbID(db, "", ""); got != "" {
		t.Errorf("empty should stay empty, got %q", got)
	}
}

// HIGH-2 end-to-end(단위): 숫자 subject 로 들어온 grace 사용자가 취소에 성공해야 한다(404 아님).
func TestGraceTokenCancelSucceeds(t *testing.T) {
	db := newLeaveTestDB(t)
	db.Exec(`CREATE TABLE v2_users (id INTEGER PRIMARY KEY, username TEXT)`)
	db.Exec(`INSERT INTO v2_users (id, username) VALUES (7, 'grace7')`)
	db.Exec(`INSERT INTO g5_member (mb_id, mb_nick, mb_leave_date, mb_leave_reason) VALUES ('grace7', '그레이스', ?, 'self')`, leaveDateAgo(2))

	// grace 토큰: username 클레임 없음, subject=숫자 "7"
	mbID := resolveMbID(db, "", "7")
	if mbID != "grace7" {
		t.Fatalf("resolveMbID should map 7 -> grace7, got %q", mbID)
	}
	if err := cancelSelfLeave(db, mbID, time.Now()); err != nil {
		t.Fatalf("cancel should succeed for grace user, got %v", err)
	}
	leave, _, _, _ := getMember(t, db, "grace7")
	if leave != "" {
		t.Errorf("leave_date should be cleared after cancel, got %q", leave)
	}
}
