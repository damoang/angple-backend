package v2

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	v2domain "github.com/damoang/angple-backend/internal/domain/v2"
	"github.com/damoang/angple-backend/internal/middleware"
	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// 회원 프로필 조회(GET /api/v2/users/:id, /username/:username) 탈퇴 가드의 계약:
//
//   - 탈퇴(숙려 포함) 회원: 없는 회원과 같은 404 응답(본문까지 같음)
//   - 일반 회원: 기존 그대로 200
//   - 판정 실패: 없는 회원과 같은 응답(비노출 우선)
//   - 숙려 중 탈퇴 취소: 다시 보인다

// fakeUserRepo 의 회원: 7=left(탈퇴), 8=normal, 그 밖=없음
type fakeUserRepo struct {
	v2repo.UserRepository
}

var fakeV2Users = map[uint64]string{7: "left", 8: "normal"}

func (fakeUserRepo) FindByID(id uint64) (*v2domain.V2User, error) {
	name, ok := fakeV2Users[id]
	if !ok {
		return &v2domain.V2User{}, gorm.ErrRecordNotFound
	}
	return &v2domain.V2User{ID: id, Username: name, Nickname: "nick-" + name, Status: "active"}, nil
}

func (fakeUserRepo) FindByUsername(username string) (*v2domain.V2User, error) {
	for id, name := range fakeV2Users {
		if name == username {
			return &v2domain.V2User{ID: id, Username: name, Nickname: "nick-" + name, Status: "active"}, nil
		}
	}
	return &v2domain.V2User{}, gorm.ErrRecordNotFound
}

func stubV2UserWithdrawn(t *testing.T, fn func(context.Context, string) (bool, error)) {
	t.Helper()
	prev := v2UserWithdrawn
	v2UserWithdrawn = fn
	t.Cleanup(func() { v2UserWithdrawn = prev })
}

func onlyLeftUserWithdrawn(_ context.Context, mbID string) (bool, error) { return mbID == "left", nil }

func doV2GetUser(byUsername bool, key string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	h := NewV2Handler(fakeUserRepo{}, nil, nil, nil, nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	if byUsername {
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v2/users/username/"+key, nil)
		c.Params = gin.Params{{Key: "username", Value: key}}
		h.GetUserByUsername(c)
		return w
	}
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v2/users/"+key, nil)
	c.Params = gin.Params{{Key: "id", Value: key}}
	h.GetUser(c)
	return w
}

func TestV2GetUser_WithdrawnLooksNotFound(t *testing.T) {
	stubV2UserWithdrawn(t, onlyLeftUserWithdrawn)

	cases := []struct {
		name       string
		byUsername bool
		left       string
		missing    string
	}{
		{name: "by id", left: "7", missing: "99"},
		{name: "by username", byUsername: true, left: "left", missing: "nobody"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			left := doV2GetUser(tc.byUsername, tc.left)
			missing := doV2GetUser(tc.byUsername, tc.missing)
			if left.Code != http.StatusNotFound {
				t.Fatalf("withdrawn status = %d, want 404", left.Code)
			}
			if left.Body.String() != missing.Body.String() {
				t.Fatalf("withdrawn response must equal not-found response:\n got  %s\n want %s", left.Body.String(), missing.Body.String())
			}
		})
	}
}

func TestV2GetUser_NormalUnchanged(t *testing.T) {
	stubV2UserWithdrawn(t, onlyLeftUserWithdrawn)

	for _, tc := range []struct {
		byUsername bool
		key        string
	}{{false, "8"}, {true, "normal"}} {
		w := doV2GetUser(tc.byUsername, tc.key)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200 (%s)", tc.key, w.Code, w.Body.String())
		}
	}
}

func TestV2GetUser_LookupFailureLooksNotFound(t *testing.T) {
	stubV2UserWithdrawn(t, func(context.Context, string) (bool, error) { return false, errors.New("db down") })

	w := doV2GetUser(false, "8")
	missing := doV2GetUser(false, "99")
	if w.Code != http.StatusNotFound || w.Body.String() != missing.Body.String() {
		t.Fatalf("lookup failure must look like not-found, got %d %s", w.Code, w.Body.String())
	}
}

// 실제 판정(middleware.IsWithdrawnMember)으로 숙려 중 취소 시 다시 보이는지 본다.
func TestV2GetUser_GraceCancelWithRealLookup(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=private"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec(`CREATE TABLE g5_member (mb_id TEXT PRIMARY KEY, mb_leave_date TEXT NOT NULL DEFAULT '')`).Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	if err := db.Exec(`INSERT INTO g5_member (mb_id, mb_leave_date) VALUES ('left', '20261013'), ('normal', '')`).Error; err != nil {
		t.Fatalf("insert: %v", err)
	}
	middleware.SetWithdrawalCheck(db, nil)
	t.Cleanup(func() { middleware.SetWithdrawalCheck(nil, nil) })

	if w := doV2GetUser(false, "7"); w.Code != http.StatusNotFound {
		t.Fatalf("during grace: status = %d, want 404", w.Code)
	}
	if w := doV2GetUser(false, "8"); w.Code != http.StatusOK {
		t.Fatalf("normal: status = %d, want 200", w.Code)
	}

	if err := db.Exec(`UPDATE g5_member SET mb_leave_date = '' WHERE mb_id = 'left'`).Error; err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if w := doV2GetUser(false, "7"); w.Code != http.StatusOK {
		t.Fatalf("after cancel: status = %d, want 200", w.Code)
	}
}
