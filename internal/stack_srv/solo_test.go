package stacksrv

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// validClaim 은 엔진 규칙상 가능한 기록이다.
// clears 1,2,4,3,4 = 14줄 → 레벨 2. 줄 점수 = 100 + 250 + 700 + 450 (레벨 1) + 700×2 (10줄째부터 레벨 2) = 2900.
// 조각 40개 → 칸 160 − 126 = 34. 낙하 점수 500 ≤ 2×30×41.
func validClaim() SoloClaim {
	return SoloClaim{Score: 3400, Lines: 14, Level: 2, Ticks: 3000, Pieces: 40, Clears: []float64{1, 2, 4, 3, 4}}
}

func hasReason(r SoloCheck, code string) bool {
	for _, c := range r.Reasons {
		if c == code {
			return true
		}
	}
	return false
}

func TestSoloClaimCheckAcceptsValid(t *testing.T) {
	r := SoloClaimCheck(validClaim(), 60000)
	if !r.OK() {
		t.Fatalf("reasons = %v", r.Reasons)
	}
	if r.ClearScore != 2900 || r.DropScore != 500 {
		t.Fatalf("clearScore=%d dropScore=%d", r.ClearScore, r.DropScore)
	}
	empty := SoloClaim{Score: 0, Lines: 0, Level: 1, Ticks: 0, Pieces: 0, Clears: []float64{}}
	if r := SoloClaimCheck(empty, 0); !r.OK() {
		t.Fatalf("빈 판 = %v", r.Reasons)
	}
}

func TestSoloClaimCheckRejectsTampered(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(c *SoloClaim)
		elapsed int64
		want    string
	}{
		{"음수 점수", func(c *SoloClaim) { c.Score = -1 }, 60000, "shape"},
		{"소수 줄", func(c *SoloClaim) { c.Lines = 14.5 }, 60000, "shape"},
		{"clears 없음", func(c *SoloClaim) { c.Clears = nil }, 60000, "shape"},
		{"한 번에 5줄", func(c *SoloClaim) { c.Clears = []float64{1, 2, 4, 3, 5} }, 60000, "clears_range"},
		{"줄 합 불일치", func(c *SoloClaim) { c.Lines = 15 }, 60000, "lines_sum"},
		{"레벨 조작", func(c *SoloClaim) { c.Level = 3 }, 60000, "level"},
		{"칸 초과", func(c *SoloClaim) { c.Pieces = 200 }, 60000, "cells"},
		{"줄을 지울 조각 부족", func(c *SoloClaim) { c.Pieces = 31 }, 60000, "cells"},
		{"조각보다 적은 틱", func(c *SoloClaim) { c.Ticks = 39 }, 60000, "pieces_ticks"},
		{"실경과보다 많은 틱", func(_ *SoloClaim) {}, 1000, "ticks_elapsed"},
		{"줄 점수보다 낮은 점수", func(c *SoloClaim) { c.Score = 2899 }, 60000, "score_low"},
		{"낙하 점수 상한 초과", func(c *SoloClaim) { c.Score = 2900 + 2*30*41 + 1 }, 60000, "drop_cap"},
	}
	for _, tc := range cases {
		c := validClaim()
		tc.mutate(&c)
		r := SoloClaimCheck(c, tc.elapsed)
		if r.OK() || !hasReason(r, tc.want) {
			t.Fatalf("%s: reasons = %v, want %s", tc.name, r.Reasons, tc.want)
		}
	}
	// 낙하 점수 상한 바로 아래는 통과한다.
	edge := validClaim()
	edge.Score = 2900 + 2*30*41
	if r := SoloClaimCheck(edge, 60000); !r.OK() {
		t.Fatalf("상한 경계 = %v", r.Reasons)
	}
	// 틱 여유는 실경과 + 180틱.
	slack := validClaim()
	slack.Ticks = 60 + 180
	slack.Pieces = 40
	if r := SoloClaimCheck(slack, 1000); hasReason(r, "ticks_elapsed") {
		t.Fatalf("틱 여유 경계 = %v", r.Reasons)
	}
	slack.Ticks = 60 + 181
	if r := SoloClaimCheck(slack, 1000); !hasReason(r, "ticks_elapsed") {
		t.Fatalf("틱 여유 초과 = %v", r.Reasons)
	}
}

func TestScoreForMatchesEngine(t *testing.T) {
	want := []int64{0, 100, 250, 450, 700}
	for n, w := range want {
		if got := ScoreFor(n, 3); got != w*3 {
			t.Fatalf("ScoreFor(%d, 3) = %d", n, got)
		}
	}
}

// fakeSoloStore 는 SoloStore 의 메모리 가짜다.
type fakeSoloStore struct {
	// mu 는 records 를 지킨다.
	mu sync.Mutex
	// records 는 저장된 판이다.
	records []SoloRecord
}

func (f *fakeSoloStore) SaveSoloRun(rec SoloRecord) (SoloStanding, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.records = append(f.records, rec)
	var best int64
	for _, r := range f.records {
		if !r.Flagged && r.Score > best {
			best = r.Score
		}
	}
	return SoloStanding{Best: best, WeekBest: best, RankWeek: 1}, nil
}

// testClock 은 테스트용 시계다.
type testClock struct {
	// mu 는 t 를 지킨다.
	mu sync.Mutex
	// t 는 현재 시각이다.
	t time.Time
}

func (c *testClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *testClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newTestSolo(store SoloStore) (*SoloService, *testClock) {
	clock := &testClock{t: time.Date(2026, 10, 10, 12, 0, 0, 0, kst)}
	verify := func(token string) (string, string, error) {
		if strings.HasPrefix(token, "member-") {
			return strings.TrimPrefix(token, "member-"), "닉", nil
		}
		return "", "", errors.New("bad token")
	}
	svc := NewSoloService(store, verify)
	svc.now = clock.now
	return svc, clock
}

func TestSoloStartRateLimitAndOneRunPerMember(t *testing.T) {
	svc, clock := newTestSolo(&fakeSoloStore{})
	first, _, err := svc.Start("p1")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Start("p1"); !errors.Is(err, errSoloRateLimited) {
		t.Fatalf("5초 안 재발급 = %v", err)
	}
	if _, _, err := svc.Start("p2"); err != nil {
		t.Fatalf("다른 회원은 막히면 안 된다: %v", err)
	}
	clock.advance(SoloStartInterval)
	second, _, err := svc.Start("p1")
	if err != nil || second == first {
		t.Fatalf("5초 뒤 재발급 = %v (%s)", err, second)
	}
	if svc.take("p1", first) != nil {
		t.Fatal("회원당 1개 — 이전 판은 버려져야 한다")
	}
	if svc.take("p2", second) != nil {
		t.Fatal("남의 판을 꺼낼 수 있으면 안 된다")
	}
	if svc.take("p1", second) == nil {
		t.Fatal("내 판을 꺼내지 못했다")
	}
	if svc.take("p1", second) != nil {
		t.Fatal("한 판은 한 번만 제출할 수 있다")
	}
}

func TestSoloRunExpires(t *testing.T) {
	svc, clock := newTestSolo(&fakeSoloStore{})
	run, _, _ := svc.Start("p1")
	clock.advance(SoloRunTTL + time.Second)
	if svc.take("p1", run) != nil {
		t.Fatal("2시간 지난 판을 받았다")
	}
}

func TestSoloFinishStoresFlaggedButDoesNotAccept(t *testing.T) {
	store := &fakeSoloStore{}
	svc, clock := newTestSolo(store)
	post := func(path, token string, body interface{}) (int, map[string]interface{}) {
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		if strings.HasSuffix(path, "start") {
			svc.HandleStart(rec, req)
		} else {
			svc.HandleFinish(rec, req)
		}
		var out map[string]interface{}
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}

	if code, _ := post("/stack-ws/solo/start", "", nil); code != http.StatusUnauthorized {
		t.Fatalf("비로그인 start = %d", code)
	}
	code, started := post("/stack-ws/solo/start", "member-p1", nil)
	if code != http.StatusOK || started["runId"] == nil || started["seed"] == nil {
		t.Fatalf("start = %d %v", code, started)
	}
	clock.advance(time.Minute)
	ok := validClaim()
	body := map[string]interface{}{"runId": started["runId"], "score": ok.Score, "lines": ok.Lines, "level": ok.Level, "ticks": ok.Ticks, "pieces": ok.Pieces, "clears": ok.Clears}
	code, res := post("/stack-ws/solo/finish", "member-p1", body)
	if code != http.StatusOK || res["accepted"] != true || res["best"] != float64(3400) {
		t.Fatalf("정상 finish = %d %v", code, res)
	}

	clock.advance(SoloStartInterval)
	_, started = post("/stack-ws/solo/start", "member-p1", nil)
	clock.advance(time.Minute)
	body["runId"] = started["runId"]
	body["score"] = 999999
	code, res = post("/stack-ws/solo/finish", "member-p1", body)
	if code != http.StatusOK || res["accepted"] != false || res["best"] != float64(3400) {
		t.Fatalf("조작 finish = %d %v", code, res)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.records) != 2 || store.records[0].Flagged || !store.records[1].Flagged || !strings.Contains(store.records[1].FlagReason, "drop_cap") {
		t.Fatalf("records = %+v", store.records)
	}
	// 같은 runId 재제출은 받지 않는다.
	code, _ = post("/stack-ws/solo/finish", "member-p1", body)
	if code != http.StatusConflict {
		t.Fatalf("재제출 = %d", code)
	}
}
