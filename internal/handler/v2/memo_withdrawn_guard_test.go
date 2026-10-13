package v2

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	v2domain "github.com/damoang/angple-backend/internal/domain/v2"
	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// v2 회원 메모 탈퇴 대상 가드의 계약 (v1 과 같은 원칙):
//
//   - 대상 v2 ID → v2_users.username(= mb_id) → 탈퇴 판정
//   - 탈퇴 대상: 조회=메모 없음과 같은 응답, 저장·수정=403, 삭제=허용
//   - 일반 대상: 기존 그대로
//   - 판정 실패: 조회=숨김, 저장=거부

type fakeMemoRepo struct {
	v2repo.MemoRepository
	memo    *v2domain.V2Memo
	upserts int
	deletes int
	findErr error
}

func (f *fakeMemoRepo) FindByUserAndTarget(uint64, uint64) (*v2domain.V2Memo, error) {
	if f.findErr != nil {
		return nil, f.findErr
	}
	if f.memo == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return f.memo, nil
}

func (f *fakeMemoRepo) Upsert(*v2domain.V2Memo) error { f.upserts++; return nil }
func (f *fakeMemoRepo) Delete(uint64, uint64) error   { f.deletes++; return nil }

// 대상 ID: 7=left(탈퇴), 8=normal, 9=v2_users 에 없음
func newV2MemoGuardHandler(t *testing.T, repo *fakeMemoRepo, withdrawn func(context.Context, string) (bool, error)) *MemoHandler {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=private"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec(`CREATE TABLE v2_users (id INTEGER PRIMARY KEY, username TEXT)`).Error; err != nil {
		t.Fatalf("create v2_users: %v", err)
	}
	if err := db.Exec(`INSERT INTO v2_users (id, username) VALUES (7, 'left'), (8, 'normal')`).Error; err != nil {
		t.Fatalf("insert v2_users: %v", err)
	}
	prev := v2MemoMemberWithdrawn
	v2MemoMemberWithdrawn = withdrawn
	t.Cleanup(func() { v2MemoMemberWithdrawn = prev })
	return NewMemoHandler(repo).WithWithdrawnTargetGuard(db)
}

func onlyLeftWithdrawn(_ context.Context, mbID string) (bool, error) { return mbID == "left", nil }

func doV2Memo(h *MemoHandler, method, targetID, body string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, "/api/v2/members/"+targetID+"/memo", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: targetID}}
	c.Set("userID", "1")
	switch method {
	case http.MethodGet:
		h.GetMemo(c)
	case http.MethodPost:
		h.CreateMemo(c)
	case http.MethodPut:
		h.UpdateMemo(c)
	case http.MethodDelete:
		h.DeleteMemo(c)
	}
	return w
}

func decodeV2(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("decode %q: %v", w.Body.String(), err)
	}
	return m
}

func TestV2Memo_GetWithdrawnLooksLikeNoMemo(t *testing.T) {
	memo := &v2domain.V2Memo{UserID: 1, TargetUserID: 7, Content: "x", Color: "yellow"}
	h := newV2MemoGuardHandler(t, &fakeMemoRepo{memo: memo}, onlyLeftWithdrawn)
	hidden := doV2Memo(h, http.MethodGet, "7", "")

	none := doV2Memo(NewMemoHandler(&fakeMemoRepo{}), http.MethodGet, "7", "")
	if hidden.Code != http.StatusOK || hidden.Body.String() != none.Body.String() {
		t.Fatalf("withdrawn GET must equal no-memo response: got %d %q, want %q", hidden.Code, hidden.Body.String(), none.Body.String())
	}
}

func TestV2Memo_GetNormalUnchanged(t *testing.T) {
	memo := &v2domain.V2Memo{UserID: 1, TargetUserID: 8, Content: "hello", Color: "yellow"}
	h := newV2MemoGuardHandler(t, &fakeMemoRepo{memo: memo}, onlyLeftWithdrawn)
	got := doV2Memo(h, http.MethodGet, "8", "")
	want := doV2Memo(NewMemoHandler(&fakeMemoRepo{memo: memo}), http.MethodGet, "8", "")
	if got.Code != http.StatusOK || got.Body.String() != want.Body.String() {
		t.Fatalf("normal GET changed: got %q, want %q", got.Body.String(), want.Body.String())
	}
	if decodeV2(t, got)["data"] == nil {
		t.Fatalf("normal GET must return the memo")
	}
}

func TestV2Memo_WriteWithdrawnRejected(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut} {
		repo := &fakeMemoRepo{}
		h := newV2MemoGuardHandler(t, repo, onlyLeftWithdrawn)
		w := doV2Memo(h, method, "7", `{"content":"x"}`)
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s withdrawn: status %d, want 403", method, w.Code)
		}
		if !strings.Contains(w.Body.String(), memoWithdrawnTargetMessage) {
			t.Fatalf("%s withdrawn: body %q", method, w.Body.String())
		}
		if repo.upserts != 0 {
			t.Fatalf("%s withdrawn: must not save", method)
		}
	}
}

func TestV2Memo_WriteNormalAndUnknownAllowed(t *testing.T) {
	for _, target := range []string{"8", "9"} {
		repo := &fakeMemoRepo{}
		h := newV2MemoGuardHandler(t, repo, onlyLeftWithdrawn)
		if w := doV2Memo(h, http.MethodPost, target, `{"content":"x"}`); w.Code != http.StatusCreated {
			t.Fatalf("POST %s: status %d", target, w.Code)
		}
		if w := doV2Memo(h, http.MethodPut, target, `{"content":"x"}`); w.Code != http.StatusOK {
			t.Fatalf("PUT %s: status %d", target, w.Code)
		}
		if repo.upserts != 2 {
			t.Fatalf("target %s: upserts=%d, want 2", target, repo.upserts)
		}
	}
}

func TestV2Memo_DeleteWithdrawnAllowed(t *testing.T) {
	repo := &fakeMemoRepo{}
	h := newV2MemoGuardHandler(t, repo, onlyLeftWithdrawn)
	if w := doV2Memo(h, http.MethodDelete, "7", ""); w.Code != http.StatusOK || repo.deletes != 1 {
		t.Fatalf("DELETE withdrawn must be allowed: status %d deletes %d", w.Code, repo.deletes)
	}
}

func TestV2Memo_GraceCancelVisibleAgain(t *testing.T) {
	withdrawn := true
	memo := &v2domain.V2Memo{UserID: 1, TargetUserID: 7, Content: "x", Color: "yellow"}
	repo := &fakeMemoRepo{memo: memo}
	h := newV2MemoGuardHandler(t, repo, func(context.Context, string) (bool, error) { return withdrawn, nil })

	if decodeV2(t, doV2Memo(h, http.MethodGet, "7", ""))["data"] != nil {
		t.Fatalf("during grace: memo must be hidden")
	}
	withdrawn = false // 숙려 중 취소
	if decodeV2(t, doV2Memo(h, http.MethodGet, "7", ""))["data"] == nil {
		t.Fatalf("after cancel: memo must be visible again")
	}
	if w := doV2Memo(h, http.MethodPost, "7", `{"content":"x"}`); w.Code != http.StatusCreated {
		t.Fatalf("after cancel: POST status %d", w.Code)
	}
}

func TestV2Memo_LookupFailure(t *testing.T) {
	memo := &v2domain.V2Memo{UserID: 1, TargetUserID: 8, Content: "x", Color: "yellow"}
	repo := &fakeMemoRepo{memo: memo}
	h := newV2MemoGuardHandler(t, repo, func(context.Context, string) (bool, error) { return false, errors.New("db down") })

	if w := doV2Memo(h, http.MethodGet, "8", ""); w.Code != http.StatusOK || decodeV2(t, w)["data"] != nil {
		t.Fatalf("lookup failure GET must be empty: %d %q", w.Code, w.Body.String())
	}
	w := doV2Memo(h, http.MethodPost, "8", `{"content":"x"}`)
	if w.Code != http.StatusServiceUnavailable || repo.upserts != 0 {
		t.Fatalf("lookup failure POST must be rejected: %d upserts=%d", w.Code, repo.upserts)
	}
	if strings.Contains(w.Body.String(), memoWithdrawnTargetMessage) {
		t.Fatalf("lookup failure must not claim the target is withdrawn")
	}
}
