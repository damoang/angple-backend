package handler

import (
	"context"
	"testing"
	"time"

	"github.com/damoang/angple-backend/internal/webcache"
)

// recordMemberCardDeletes 는 webcache 삭제 백엔드를 기록용으로 바꿔 끼운다.
func recordMemberCardDeletes(t *testing.T) *[]string {
	t.Helper()
	var keys []string
	restore := webcache.SetDeleterForTest(func(_ context.Context, ks ...string) error {
		keys = append(keys, ks...)
		return nil
	})
	t.Cleanup(restore)
	return &keys
}

func containsKey(keys []string, want string) bool {
	for _, k := range keys {
		if k == want {
			return true
		}
	}
	return false
}

// 본인 탈퇴 성공 뒤 회원 표시 캐시 키를 지운다.
func TestApplySelfLeaveInvalidatesMemberCard(t *testing.T) {
	t.Setenv("AUTH_CACHE_NAMESPACES", "")
	deleted := recordMemberCardDeletes(t)
	db := newLeaveTestDB(t)
	db.Exec(`INSERT INTO g5_member (mb_id, mb_nick, mb_leave_date, mb_leave_reason, mb_intercept_date)
		VALUES ('Dave', '데이브', '', '', '')`)

	if _, err := applySelfLeave(db, "Dave", "", time.Now()); err != nil {
		t.Fatalf("applySelfLeave: %v", err)
	}
	if !containsKey(*deleted, "mbcard:dave") || !containsKey(*deleted, "canary:mbcard:dave") {
		t.Fatalf("mbcard keys not deleted: %v", *deleted)
	}
}

// 회원 조회가 실패하면(쓰기 없음) 캐시 키를 지우지 않는다.
func TestApplySelfLeaveMissingMemberDoesNotInvalidate(t *testing.T) {
	deleted := recordMemberCardDeletes(t)
	db := newLeaveTestDB(t)

	if _, err := applySelfLeave(db, "nobody", "", time.Now()); err == nil {
		t.Fatal("expected error for missing member")
	}
	if len(*deleted) != 0 {
		t.Fatalf("no write happened, but keys were deleted: %v", *deleted)
	}
}

// 탈퇴 취소가 거부되면(쓰기 없음) 캐시 키를 지우지 않는다.
func TestCancelSelfLeaveRefusedDoesNotInvalidate(t *testing.T) {
	deleted := recordMemberCardDeletes(t)
	db := newLeaveTestDB(t)
	db.Exec(`INSERT INTO g5_member (mb_id, mb_nick, mb_leave_date, mb_leave_reason, mb_intercept_date)
		VALUES ('erin', '에린', ?, 'self', '')`, leaveDateAgo(3))

	if err := cancelSelfLeave(db, "erin", time.Now()); err == nil {
		t.Fatal("cancel must be refused")
	}
	if len(*deleted) != 0 {
		t.Fatalf("refused cancel must not delete keys: %v", *deleted)
	}
}
