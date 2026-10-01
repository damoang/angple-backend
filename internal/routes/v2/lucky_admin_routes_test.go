package v2

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	v2handler "github.com/damoang/angple-backend/internal/handler/v2"
	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
	v2svc "github.com/damoang/angple-backend/internal/service/v2"
	"github.com/damoang/angple-backend/pkg/jwt"
	"github.com/gin-gonic/gin"
)

// ⛔ 이 파일이 지키는 계약 (앙팡 관리자 라우트):
//
//   - A1 토큰 없으면 401, 관리자 아닌 회원(mb_level<10)은 403 — 네 엔드포인트 모두. 관리자는 통과.
//   - A2 HTTP 단에서도 알 수 없는 키는 400 + 필드별 메시지이고, 저장을 부르지 않는다. 이력의 by 는 토큰의 mb_id 다.
//
// 저장소는 가짜다(DB 동작은 저장소 테스트가 본다). 값은 임의의 테스트 값이다.

type routeFakeLuckyAdminRepo struct {
	savedBy []string
}

func (f *routeFakeLuckyAdminRepo) GetStoredLuckyConfig() (*v2repo.LuckyConfig, []v2repo.LuckyConfigHistoryEntry, json.RawMessage, error) {
	return v2repo.DefaultLuckyConfig(), nil, nil, nil
}

func (f *routeFakeLuckyAdminRepo) SaveLuckyConfig(_ *v2repo.LuckyConfig, by string, _ time.Time) error {
	f.savedBy = append(f.savedBy, by)
	return nil
}

func (f *routeFakeLuckyAdminRepo) ListBoardLucky() ([]v2repo.LuckyBoardSetting, error) {
	return []v2repo.LuckyBoardSetting{}, nil
}

func (f *routeFakeLuckyAdminRepo) ExistingBoardIDs(ids []string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}

func (f *routeFakeLuckyAdminRepo) SetBoardLucky(_ []string, _ v2repo.BoardLucky, _ time.Time) (map[string]string, error) {
	return map[string]string{}, nil
}

func (f *routeFakeLuckyAdminRepo) ListLuckyGrantsBetween(_, _ time.Time) ([]v2repo.LuckyGrantDetail, error) {
	return nil, nil
}

func newLuckyAdminTestRouter(t *testing.T) (*gin.Engine, *jwt.Manager, *routeFakeLuckyAdminRepo) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	mgr := jwt.NewManager("test-only-secret", 600, 3600)
	repo := &routeFakeLuckyAdminRepo{}
	r := gin.New()
	SetupAdminLucky(r, v2handler.NewLuckyAdminHandler(v2svc.NewLuckyAdminService(repo)), mgr)
	return r, mgr, repo
}

func luckyAdminDo(r *gin.Engine, method, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

var luckyAdminEndpoints = []struct{ method, path, body string }{
	{http.MethodGet, "/api/v2/admin/lucky/config", ""},
	{http.MethodPut, "/api/v2/admin/lucky/config", `{"enabled":true}`},
	{http.MethodPut, "/api/v2/admin/lucky/boards", `{"board_ids":["board_a"],"lucky":{"enabled":true,"odds":3,"points":5}}`},
	{http.MethodGet, "/api/v2/admin/lucky/stats?date=2026-10-01", ""},
}

// TestLuckyAdminRoutes_RequireAdmin — A1: 무토큰 401, 일반 회원 403, 관리자 200.
func TestLuckyAdminRoutes_RequireAdmin(t *testing.T) {
	r, mgr, repo := newLuckyAdminTestRouter(t)
	member, err := mgr.GenerateAccessToken("1", "member_a", "회원", 9)
	if err != nil {
		t.Fatalf("토큰 생성 실패: %v", err)
	}
	admin, err := mgr.GenerateAccessToken("2", "admin_a", "관리자", 10)
	if err != nil {
		t.Fatalf("토큰 생성 실패: %v", err)
	}
	for _, ep := range luckyAdminEndpoints {
		if w := luckyAdminDo(r, ep.method, ep.path, "", ep.body); w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s 무토큰: got %d want 401", ep.method, ep.path, w.Code)
		}
		if w := luckyAdminDo(r, ep.method, ep.path, member, ep.body); w.Code != http.StatusForbidden {
			t.Errorf("%s %s 일반 회원: got %d want 403", ep.method, ep.path, w.Code)
		}
		if w := luckyAdminDo(r, ep.method, ep.path, admin, ep.body); w.Code != http.StatusOK {
			t.Errorf("%s %s 관리자: got %d want 200 (%s)", ep.method, ep.path, w.Code, w.Body.String())
		}
	}
	if len(repo.savedBy) != 1 || repo.savedBy[0] != "admin_a" {
		t.Errorf("관리자 저장 1회, by=mb_id 여야 한다: %v", repo.savedBy)
	}
}

// TestLuckyAdminRoutes_UnknownKey400 — A2: 알 수 없는 키·범위 위반은 400 + 필드별 메시지, 저장 없음.
func TestLuckyAdminRoutes_UnknownKey400(t *testing.T) {
	r, mgr, repo := newLuckyAdminTestRouter(t)
	admin, err := mgr.GenerateAccessToken("2", "admin_a", "관리자", 10)
	if err != nil {
		t.Fatalf("토큰 생성 실패: %v", err)
	}
	cases := []struct{ path, body, field string }{
		{"/api/v2/admin/lucky/config", `{"enabled":true,"dailycap":3}`, "dailycap"},
		{"/api/v2/admin/lucky/config", `{"fixed_windows":[{"name":"가","start":"25:00","end":"01:00","odds":1,"points":1}]}`, "fixed_windows[0].start"},
		{"/api/v2/admin/lucky/boards", `{"board_ids":["board_a"],"lucky":{"enabled":true,"odds":3,"pts":5}}`, "pts"},
	}
	for _, c := range cases {
		w := luckyAdminDo(r, http.MethodPut, c.path, admin, c.body)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s %s: got %d want 400", c.path, c.body, w.Code)
			continue
		}
		var resp struct {
			Success bool `json:"success"`
			Error   struct {
				Details []struct {
					Field   string `json:"field"`
					Message string `json:"message"`
				} `json:"details"`
			} `json:"error"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("응답 파싱 실패: %v", err)
		}
		found := false
		for _, d := range resp.Error.Details {
			if d.Field == c.field && d.Message != "" {
				found = true
			}
		}
		if resp.Success || !found {
			t.Errorf("%s: 필드 %q 메시지가 있어야 한다: %s", c.body, c.field, w.Body.String())
		}
	}
	if len(repo.savedBy) != 0 {
		t.Errorf("거부된 요청은 저장하면 안 된다: %v", repo.savedBy)
	}
}
