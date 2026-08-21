package v2

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
)

// ⛔ 이 파일이 지키는 계약:
//
//	getBlockedMbIDs 는 조회 실패를 **nil 로 뭉개지 않는다.**
//
// 반환값이 「차단 목록」이라 nil 은 「차단 없음」으로 읽힌다. 그러면 차단한 사람의 글이
// 그대로 보인다. 이 함수를 쓰는 화면이 셋이다 — 글 목록 · 댓글 목록 · 앱 피드.
//
// 예전 코드는 `if err != nil { return nil }` 이라 **조회 실패와 차단 없음이 같은 값**이었다.
// ⭐ 판별 기준은 `!= nil` 이 아니라 **`err == nil` 인데 값이 비어 있을 수 있는가** 다.
//
// ⚠️ 「비로그인」은 실패가 아니다. 차단이 없는 게 정답이므로 (nil, nil) 이 맞다.
// 그 둘을 구분하는지도 함께 본다.

// fakeBlockRepo 는 GetContentBlockedUserIDs 만 흉내 낸다.
// 나머지는 인터페이스를 임베드해 비워 둔다 — 호출되면 panic 이 나고, 그건
// "테스트가 의도보다 넓은 경로를 탔다"는 신호라 오히려 알아야 할 정보다.
type fakeBlockRepo struct {
	v2repo.BlockRepository
	ids []string
	err error
}

func (f *fakeBlockRepo) GetContentBlockedUserIDs(string) ([]string, error) { return f.ids, f.err }

func newBlockTestCtx() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)
	return c
}

func TestGetBlockedMbIDs_PropagatesError(t *testing.T) {
	h := &V2Handler{blockRepo: &fakeBlockRepo{err: errors.New("db down")}}

	ids, err := h.getBlockedMbIDs(newBlockTestCtx(), "someone")

	// ⛔ 여기가 핵심이다. err == nil && ids == nil 이면 호출부가 "차단 없음"으로 읽고
	//    차단한 사람의 글을 그대로 내보낸다.
	if err == nil {
		t.Fatalf("error 를 기대했다. ids=%v — 이 조합이 차단 누수를 만든다", ids)
	}
	if ids != nil {
		t.Errorf("실패 시에는 목록을 주면 안 된다. ids=%v", ids)
	}
}

// TestGetBlockedMbIDs_GuestIsNotAnError 는 「비로그인」과 「조회 실패」를 구분하는지 본다.
// 비로그인은 차단이 없는 게 정답이므로 error 가 아니다.
func TestGetBlockedMbIDs_GuestIsNotAnError(t *testing.T) {
	for _, tc := range []struct {
		name string
		h    *V2Handler
		mbID string
	}{
		{"비로그인", &V2Handler{blockRepo: &fakeBlockRepo{err: errors.New("불려서는 안 된다")}}, ""},
		{"차단 기능 미구성", &V2Handler{}, "someone"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ids, err := tc.h.getBlockedMbIDs(newBlockTestCtx(), tc.mbID)
			if err != nil {
				t.Fatalf("실패가 아니어야 한다: %v", err)
			}
			if len(ids) != 0 {
				t.Errorf("차단 없음이어야 한다. ids=%v", ids)
			}
		})
	}
}

func TestGetBlockedMbIDs_ReturnsList(t *testing.T) {
	want := []string{"a", "b"}
	h := &V2Handler{blockRepo: &fakeBlockRepo{ids: want}}

	ids, err := h.getBlockedMbIDs(newBlockTestCtx(), "someone")
	if err != nil {
		t.Fatalf("정상 경로에서 error: %v", err)
	}
	if len(ids) != len(want) || ids[0] != want[0] || ids[1] != want[1] {
		t.Errorf("목록이 그대로 나와야 한다. got=%v want=%v", ids, want)
	}
}
