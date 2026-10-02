package v2

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
	v2svc "github.com/damoang/angple-backend/internal/service/v2"
)

// ⛔ 이 파일이 지키는 계약 (관리자 API 의 평소 단계 이름 base_name, HTTP 경계):
//
//   - N3 base_name 위반 PUT 은 400 + 필드별 사유(details[].field)이고 저장하지 않는다.
//   - N5 정상 PUT 한 base_name 이 GET 의 data.config.base_name 으로 그대로 돌아온다(관리자 왕복 저장).
//
// 이름 「앙팡」 외의 값은 모두 임의의 테스트 값이다.

// fakeBaseNameAdminRepo 는 설정 읽기·저장만 흉내 낸다. 저장한 값을 다음 GET 이 그대로 읽는다.
// 나머지 메서드는 인터페이스 임베드로 비워 둔다 — 호출되면 panic 이고, 그건 테스트가 의도보다 넓은 경로를 탔다는 신호다.
type fakeBaseNameAdminRepo struct {
	v2repo.LuckyAdminRepository
	cfg   *v2repo.LuckyConfig
	saves int
}

func (f *fakeBaseNameAdminRepo) GetStoredLuckyConfig() (*v2repo.LuckyConfig, []v2repo.LuckyConfigHistoryEntry, json.RawMessage, error) {
	if f.cfg == nil {
		return v2repo.DefaultLuckyConfig(), nil, nil, nil
	}
	cp := *f.cfg
	return &cp, nil, nil, nil
}

func (f *fakeBaseNameAdminRepo) SaveLuckyConfig(cfg *v2repo.LuckyConfig, _ string, _ time.Time) error {
	f.saves++
	cp := *cfg
	f.cfg = &cp
	return nil
}

func (f *fakeBaseNameAdminRepo) ListBoardLucky() ([]v2repo.LuckyBoardSetting, error) {
	return nil, nil
}

func newBaseNameAdminRouter(repo *fakeBaseNameAdminRepo) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewLuckyAdminHandler(v2svc.NewLuckyAdminService(repo))
	r := gin.New()
	r.GET("/config", h.GetConfig)
	r.PUT("/config", h.PutConfig)
	return r
}

func doLuckyAdmin(t *testing.T, r *gin.Engine, method, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, "/config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("응답 JSON 파싱 실패: %v (%s)", err, w.Body.String())
	}
	return w.Code, out
}

// TestLuckyAdminHandler_BaseNameInvalid400 — base_name 위반은 400 + details 에 그 필드, 저장 없음.
func TestLuckyAdminHandler_BaseNameInvalid400(t *testing.T) {
	cases := []struct {
		body  string
		field string
	}{
		{`{"base_name":""}`, "base_name"},
		{`{"base_name":"앙복타임"}`, "base_name"},
		{`{"base_name":"나리야"}`, "base_name"},
		{`{"base_name":"임의 럭키 이름"}`, "base_name"},
		{`{"base_name":"` + strings.Repeat("가", 21) + `"}`, "base_name"},
		{`{"base_name":"겹침가","fixed_windows":[{"name":"겹침가","start":"01:00","end":"02:00","odds":3,"points":4}]}`, "fixed_windows[0].name"},
	}
	for _, c := range cases {
		repo := &fakeBaseNameAdminRepo{}
		code, out := doLuckyAdmin(t, newBaseNameAdminRouter(repo), http.MethodPut, c.body)
		if code != http.StatusBadRequest {
			t.Errorf("%s: 400 이어야 한다, got %d", c.body, code)
			continue
		}
		raw, _ := json.Marshal(out["error"])
		if !strings.Contains(string(raw), `"field":"`+c.field+`"`) {
			t.Errorf("%s: details 에 %s 가 있어야 한다: %s", c.body, c.field, raw)
		}
		if repo.saves != 0 {
			t.Errorf("%s: 위반이면 저장하지 않는다", c.body)
		}
	}
}

// TestLuckyAdminHandler_BaseNameRoundTrip — GET 기본값 「앙팡」 → PUT 임의 이름 → GET 이 그 이름.
func TestLuckyAdminHandler_BaseNameRoundTrip(t *testing.T) {
	repo := &fakeBaseNameAdminRepo{}
	r := newBaseNameAdminRouter(repo)
	baseOf := func(out map[string]any) any {
		data, _ := out["data"].(map[string]any)
		cfg, _ := data["config"].(map[string]any)
		return cfg["base_name"]
	}

	code, out := doLuckyAdmin(t, r, http.MethodGet, "")
	if code != http.StatusOK || baseOf(out) != "앙팡" {
		t.Fatalf("미설정 GET 은 base_name 「앙팡」: code=%d out=%v", code, out)
	}

	code, out = doLuckyAdmin(t, r, http.MethodPut, `{"enabled":true,"base_name":"임의평소"}`)
	if code != http.StatusOK || baseOf(out) != "임의평소" || repo.saves != 1 {
		t.Fatalf("정상 PUT: code=%d out=%v saves=%d", code, out, repo.saves)
	}

	code, out = doLuckyAdmin(t, r, http.MethodGet, "")
	if code != http.StatusOK || baseOf(out) != "임의평소" {
		t.Fatalf("저장한 base_name 이 GET 에 보여야 한다: code=%d out=%v", code, out)
	}
}
