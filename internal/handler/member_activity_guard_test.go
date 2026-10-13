package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/damoang/angple-backend/internal/domain/gnuboard"
	"github.com/damoang/angple-backend/internal/middleware"
	gnurepo "github.com/damoang/angple-backend/internal/repository/gnuboard"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// 회원 활동(GET /api/v1/members/:id/activity) 탈퇴 가드의 계약:
//
//   - 탈퇴(숙려 포함) 회원: 저장소를 건드리지 않고 「활동 없음」과 같은 응답(200, 빈 목록)
//   - 일반 회원: 가드를 지나 기존 조회 경로를 그대로 탄다
//   - 판정 실패: 빈 응답(비노출 우선)
//   - 숙려 중 탈퇴 취소: 다시 보인다

const emptyActivityBody = `{"recentComments":[],"recentPosts":[]}`

// countingBoardRepo 는 회원 활동 조회가 저장소까지 갔는지 센다.
// 게시판 목록 조회를 실패시켜 이후 경로는 「활동 없음」 응답으로 끝난다.
type countingBoardRepo struct {
	gnurepo.BoardRepository
	findAll int
}

func (f *countingBoardRepo) FindAll() ([]*gnuboard.G5Board, error) {
	f.findAll++
	return nil, errors.New("board list unavailable in test")
}

func stubMemberActivityWithdrawn(t *testing.T, fn func(context.Context, string) (bool, error)) {
	t.Helper()
	prev := memberActivityWithdrawn
	memberActivityWithdrawn = fn
	t.Cleanup(func() { memberActivityWithdrawn = prev })
}

func doMemberActivity(t *testing.T, mbID string) (*httptest.ResponseRecorder, *countingBoardRepo) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	boards := &countingBoardRepo{}
	h := NewMyPageHandler(gnurepo.NewMyPageRepository(nil, boards))
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/members/"+mbID+"/activity", nil)
	c.Params = gin.Params{{Key: "id", Value: mbID}}
	h.GetMemberActivity(c)
	return w, boards
}

func TestMemberActivity_WithdrawnReturnsEmpty(t *testing.T) {
	stubMemberActivityWithdrawn(t, func(_ context.Context, id string) (bool, error) { return id == "left", nil })

	w, boards := doMemberActivity(t, "left")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := w.Body.String(); got != emptyActivityBody {
		t.Fatalf("body = %s, want %s", got, emptyActivityBody)
	}
	if boards.findAll != 0 {
		t.Fatalf("withdrawn member must not reach the repository (calls=%d)", boards.findAll)
	}
}

func TestMemberActivity_NormalPassesThrough(t *testing.T) {
	stubMemberActivityWithdrawn(t, func(_ context.Context, id string) (bool, error) { return id == "left", nil })

	w, boards := doMemberActivity(t, "normal")
	if boards.findAll != 1 {
		t.Fatalf("normal member must reach the repository once (calls=%d)", boards.findAll)
	}
	// 탈퇴 회원 응답은 「활동 없음」 응답과 구별되지 않아야 한다.
	if w.Code != http.StatusOK || w.Body.String() != emptyActivityBody {
		t.Fatalf("no-activity response = %d %s", w.Code, w.Body.String())
	}
}

func TestMemberActivity_LookupFailureReturnsEmpty(t *testing.T) {
	stubMemberActivityWithdrawn(t, func(context.Context, string) (bool, error) { return false, errors.New("db down") })

	w, boards := doMemberActivity(t, "anyone")
	if w.Code != http.StatusOK || w.Body.String() != emptyActivityBody {
		t.Fatalf("lookup failure must return empty activity, got %d %s", w.Code, w.Body.String())
	}
	if boards.findAll != 0 {
		t.Fatalf("lookup failure must not reach the repository (calls=%d)", boards.findAll)
	}
}

// 실제 판정(middleware.IsWithdrawnMember)으로 숙려 중 취소 시 다시 보이는지 본다.
func TestMemberActivity_GraceCancelWithRealLookup(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=private"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec(`CREATE TABLE g5_member (mb_id TEXT PRIMARY KEY, mb_leave_date TEXT NOT NULL DEFAULT '')`).Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	if err := db.Exec(`INSERT INTO g5_member (mb_id, mb_leave_date) VALUES ('grace', '20261013'), ('normal', '')`).Error; err != nil {
		t.Fatalf("insert: %v", err)
	}
	middleware.SetWithdrawalCheck(db, nil)
	t.Cleanup(func() { middleware.SetWithdrawalCheck(nil, nil) })

	if !hideMemberActivity(context.Background(), "grace") {
		t.Fatalf("during grace: activity must be hidden")
	}
	if hideMemberActivity(context.Background(), "normal") {
		t.Fatalf("normal member: activity must be visible")
	}
	if _, boards := doMemberActivity(t, "grace"); boards.findAll != 0 {
		t.Fatalf("during grace: handler must not reach the repository")
	}

	if err := db.Exec(`UPDATE g5_member SET mb_leave_date = '' WHERE mb_id = 'grace'`).Error; err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if hideMemberActivity(context.Background(), "grace") {
		t.Fatalf("after cancel: activity must be visible again")
	}
}
