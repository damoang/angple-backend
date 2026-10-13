package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// 회원 메모 탈퇴 대상 가드의 계약:
//
//   - 탈퇴(숙려 포함) 대상: 조회=숨김(메모 없음과 같은 응답), 저장·수정=403
//   - 일반 대상: 그대로 통과(응답 변화 없음)
//   - 숙려 중 탈퇴 취소: 다시 보이고 저장 가능
//   - 판정 실패: 조회=숨김(빈 결과), 저장=거부 — 실패가 다른 회원 메모까지 노출시키지 않는다

func stubMemoTargetWithdrawn(t *testing.T, fn func(context.Context, string) (bool, error)) {
	t.Helper()
	prev := memoTargetWithdrawn
	memoTargetWithdrawn = fn
	t.Cleanup(func() { memoTargetWithdrawn = prev })
}

func stubMemoWithdrawnTargetIDs(t *testing.T, fn func(context.Context, []string) (map[string]bool, error)) *int {
	t.Helper()
	calls := 0
	prev := memoWithdrawnTargetIDs
	memoWithdrawnTargetIDs = func(ctx context.Context, ids []string) (map[string]bool, error) {
		calls++
		return fn(ctx, ids)
	}
	t.Cleanup(func() { memoWithdrawnTargetIDs = prev })
	return &calls
}

func withdrawnSet(ids ...string) func(context.Context, string) (bool, error) {
	set := map[string]bool{}
	for _, id := range ids {
		set[id] = true
	}
	return func(_ context.Context, id string) (bool, error) { return set[id], nil }
}

func newMemoGuardCtx() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	return c, w
}

func TestHideMemoForTarget(t *testing.T) {
	stubMemoTargetWithdrawn(t, withdrawnSet("left"))
	if !HideMemoForTarget(context.Background(), "left") {
		t.Errorf("withdrawn target must be hidden")
	}
	if HideMemoForTarget(context.Background(), "normal") {
		t.Errorf("normal target must not be hidden")
	}
}

func TestHideMemoForTarget_LookupFailureHides(t *testing.T) {
	stubMemoTargetWithdrawn(t, func(context.Context, string) (bool, error) { return false, errors.New("db down") })
	if !HideMemoForTarget(context.Background(), "anyone") {
		t.Errorf("lookup failure must hide (non-exposure first)")
	}
}

func TestRejectMemoWriteForTarget_Withdrawn403(t *testing.T) {
	stubMemoTargetWithdrawn(t, withdrawnSet("left"))
	c, w := newMemoGuardCtx()
	if !RejectMemoWriteForTarget(c, "left") {
		t.Fatalf("withdrawn target must be rejected")
	}
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
	var body struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Success || body.Error != MemoWithdrawnTargetMessage {
		t.Fatalf("body = %+v", body)
	}
}

func TestRejectMemoWriteForTarget_NormalPasses(t *testing.T) {
	stubMemoTargetWithdrawn(t, withdrawnSet("left"))
	c, w := newMemoGuardCtx()
	if RejectMemoWriteForTarget(c, "normal") {
		t.Fatalf("normal target must not be rejected")
	}
	if w.Body.Len() != 0 {
		t.Fatalf("guard must not write a response for normal target, got %q", w.Body.String())
	}
}

func TestRejectMemoWriteForTarget_LookupFailureRejects(t *testing.T) {
	stubMemoTargetWithdrawn(t, func(context.Context, string) (bool, error) { return false, errors.New("db down") })
	c, w := newMemoGuardCtx()
	if !RejectMemoWriteForTarget(c, "anyone") {
		t.Fatalf("lookup failure must reject writes")
	}
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
	if strings.Contains(w.Body.String(), MemoWithdrawnTargetMessage) {
		t.Fatalf("lookup failure must not claim the target is withdrawn")
	}
}

func TestDropWithdrawnMemoTargets(t *testing.T) {
	var gotIDs []string
	calls := stubMemoWithdrawnTargetIDs(t, func(_ context.Context, ids []string) (map[string]bool, error) {
		gotIDs = ids
		return map[string]bool{"left": true}, nil
	})
	in := map[string]interface{}{"left": "m1", "normal": "m2"}
	out := DropWithdrawnMemoTargets(context.Background(), in)
	if *calls != 1 || len(gotIDs) != 2 {
		t.Fatalf("expected one IN lookup with 2 ids, calls=%d ids=%v", *calls, gotIDs)
	}
	if !reflect.DeepEqual(out, map[string]interface{}{"normal": "m2"}) {
		t.Fatalf("got %v", out)
	}
}

func TestDropWithdrawnMemoTargets_EmptySkipsLookup(t *testing.T) {
	calls := stubMemoWithdrawnTargetIDs(t, func(context.Context, []string) (map[string]bool, error) {
		return nil, nil
	})
	out := DropWithdrawnMemoTargets(context.Background(), map[string]interface{}{})
	if *calls != 0 || len(out) != 0 {
		t.Fatalf("empty input must not query, calls=%d out=%v", *calls, out)
	}
}

func TestDropWithdrawnMemoTargets_LookupFailureEmpty(t *testing.T) {
	stubMemoWithdrawnTargetIDs(t, func(context.Context, []string) (map[string]bool, error) {
		return nil, errors.New("db down")
	})
	out := DropWithdrawnMemoTargets(context.Background(), map[string]interface{}{"a": 1, "b": 2})
	if out == nil || len(out) != 0 {
		t.Fatalf("lookup failure must return empty (non-nil) map, got %v", out)
	}
}

func TestDropWithdrawnFromMemoTargets_KeepsOrder(t *testing.T) {
	calls := stubMemoWithdrawnTargetIDs(t, func(context.Context, []string) (map[string]bool, error) {
		return map[string]bool{"b": true, "d": true}, nil
	})
	in := MemoTargets{Targets: []string{"a", "b", "c", "d", "e"}, Count: 5, Truncated: true}
	out := DropWithdrawnFromMemoTargets(context.Background(), in)
	if *calls != 1 {
		t.Fatalf("expected one lookup, got %d", *calls)
	}
	if !reflect.DeepEqual(out.Targets, []string{"a", "c", "e"}) || out.Count != 3 || !out.Truncated {
		t.Fatalf("got %+v", out)
	}
}

func TestDropWithdrawnFromMemoTargets_NoneWithdrawnUnchanged(t *testing.T) {
	stubMemoWithdrawnTargetIDs(t, func(context.Context, []string) (map[string]bool, error) {
		return map[string]bool{}, nil
	})
	in := MemoTargets{Targets: []string{"a", "b"}, Count: 2}
	out := DropWithdrawnFromMemoTargets(context.Background(), in)
	if !reflect.DeepEqual(out, in) {
		t.Fatalf("got %+v, want %+v", out, in)
	}
}

func TestDropWithdrawnFromMemoTargets_LookupFailureEmpty(t *testing.T) {
	stubMemoWithdrawnTargetIDs(t, func(context.Context, []string) (map[string]bool, error) {
		return nil, errors.New("db down")
	})
	out := DropWithdrawnFromMemoTargets(context.Background(), MemoTargets{Targets: []string{"a"}, Count: 1})
	if out.Targets == nil || len(out.Targets) != 0 || out.Count != 0 || out.Truncated {
		t.Fatalf("lookup failure must return empty list (targets [] not null), got %+v", out)
	}
}

// 실제 판정(middleware.IsWithdrawnMember)으로 숙려 중 취소 시 다시 저장·조회되는지 본다.
func TestMemoGuard_GraceCancelWithRealLookup(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=private"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec(`CREATE TABLE g5_member (mb_id TEXT PRIMARY KEY, mb_leave_date TEXT NOT NULL DEFAULT '')`).Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	if err := db.Exec(`INSERT INTO g5_member (mb_id, mb_leave_date) VALUES ('grace', '20261013')`).Error; err != nil {
		t.Fatalf("insert: %v", err)
	}
	middleware.SetWithdrawalCheck(db, nil)
	t.Cleanup(func() { middleware.SetWithdrawalCheck(nil, nil) })

	if !HideMemoForTarget(context.Background(), "grace") {
		t.Fatalf("during grace: memo must be hidden")
	}
	c, w := newMemoGuardCtx()
	if !RejectMemoWriteForTarget(c, "grace") || w.Code != http.StatusForbidden {
		t.Fatalf("during grace: write must be 403, got %d", w.Code)
	}

	if err := db.Exec(`UPDATE g5_member SET mb_leave_date = '' WHERE mb_id = 'grace'`).Error; err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if HideMemoForTarget(context.Background(), "grace") {
		t.Fatalf("after cancel: memo must be visible again")
	}
	c2, _ := newMemoGuardCtx()
	if RejectMemoWriteForTarget(c2, "grace") {
		t.Fatalf("after cancel: write must be allowed")
	}
}
