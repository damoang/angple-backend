package stacksrv

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestWeekStartKST(t *testing.T) {
	cases := []struct {
		at   time.Time
		want string
	}{
		// 2026-10-11 은 일요일. 일요일 23:59 KST 는 아직 지난주(10-05 월요일 시작).
		{time.Date(2026, 10, 11, 23, 59, 0, 0, kst), "2026-10-05"},
		// 월요일 0시 KST = 일요일 15:00 UTC 부터 새 주.
		{time.Date(2026, 10, 11, 15, 0, 0, 0, time.UTC), "2026-10-12"},
		{time.Date(2026, 10, 11, 14, 59, 59, 0, time.UTC), "2026-10-05"},
		{time.Date(2026, 10, 14, 9, 0, 0, 0, kst), "2026-10-12"},
	}
	for _, c := range cases {
		got := WeekStartKST(c.at)
		if got.Format("2006-01-02") != c.want || got.Hour() != 0 || got.Weekday() != time.Monday {
			t.Fatalf("WeekStartKST(%v) = %v, want %s", c.at, got, c.want)
		}
	}
}

// fakeBoardStore 는 LeaderboardStore 의 가짜다. 호출 횟수를 센다.
type fakeBoardStore struct {
	// mu 는 calls 를 지킨다.
	mu sync.Mutex
	// calls 는 조회 횟수다.
	calls int
	// lastN 은 마지막으로 요청받은 인원이다.
	lastN int
	// lastRule 은 VersusTop 에 마지막으로 넘어온 규칙이다.
	lastRule string
}

func (f *fakeBoardStore) SoloTop(_ time.Time, n int) ([]SoloRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastN = n
	return []SoloRow{
		{Nickname: "첫째", Score: 12345, AchievedRaw: "2026-10-10 00:05:00"},
		{Nickname: "둘째", Score: 100},
	}, nil
}

func (f *fakeBoardStore) VersusTop(rule string, n int) ([]VersusRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastN = n
	f.lastRule = rule
	return []VersusRow{{Nickname: "고수", Rating: 1620, Wins: 3}}, nil
}

func (f *fakeBoardStore) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func TestLeaderboardCachesAndHidesMemberID(t *testing.T) {
	store := &fakeBoardStore{}
	lb := NewLeaderboard(store)
	clock := &testClock{t: time.Date(2026, 10, 10, 12, 0, 0, 0, kst)}
	lb.now = clock.now
	get := func(board string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		lb.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/stack-ws/leaderboard?board="+board, nil))
		return rec
	}

	rec := get(BoardSoloWeek)
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "public, max-age=60" {
		t.Fatalf("code=%d cache=%q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	body := rec.Body.String()
	if strings.Contains(body, "mb_id") || strings.Contains(body, "mbId") {
		t.Fatalf("회원 아이디가 공개 응답에 실렸다: %s", body)
	}
	var out struct {
		Board     string    `json:"board"`
		WeekStart string    `json:"weekStart"`
		Rows      []SoloRow `json:"rows"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.WeekStart != "2026-10-05" || len(out.Rows) != 2 || out.Rows[0].Rank != 1 || out.Rows[1].Rank != 2 {
		t.Fatalf("out = %+v", out)
	}
	if out.Rows[0].AchievedAt != "2026-10-10T00:05:00+09:00" || out.Rows[1].AchievedAt != "" {
		t.Fatalf("achievedAt = %q, %q", out.Rows[0].AchievedAt, out.Rows[1].AchievedAt)
	}
	if strings.Contains(body, "achieved_raw") || strings.Contains(body, "AchievedRaw") || !strings.Contains(body, `"achievedAt":""`) {
		t.Fatalf("응답 모양이 다르다: %s", body)
	}
	if store.lastN != 10 {
		t.Fatalf("점수판은 10줄이어야 한다 (%d)", store.lastN)
	}

	get(BoardSoloWeek)
	clock.advance(30 * time.Second)
	get(BoardSoloWeek)
	if n := store.count(); n != 1 {
		t.Fatalf("60초 안에 저장소를 %d번 읽었다", n)
	}
	clock.advance(31 * time.Second)
	get(BoardSoloWeek)
	if n := store.count(); n != 2 {
		t.Fatalf("60초 뒤에는 새로 읽어야 한다 (%d)", n)
	}
	get(BoardVsAttack)
	get(BoardVsSprint40)
	get(BoardSoloAll)
	if n := store.count(); n != 5 {
		t.Fatalf("보드별로 따로 캐시해야 한다 (%d)", n)
	}
	if store.lastN != LeaderboardSize || LeaderboardSize != 10 {
		t.Fatalf("대전 보드도 10줄이어야 한다 (%d)", store.lastN)
	}
	if rec := get("vs_score60"); rec.Code != http.StatusBadRequest {
		t.Fatalf("알 수 없는 보드 = %d", rec.Code)
	}
}

func TestLeaderboardVersusBoardRules(t *testing.T) {
	store := &fakeBoardStore{}
	lb := NewLeaderboard(store)
	lb.now = (&testClock{t: time.Date(2026, 10, 12, 12, 0, 0, 0, kst)}).now
	cases := []struct {
		board string
		rule  string
	}{
		{BoardVsAttack, RuleAttack},
		{BoardVsSprint40, RuleSprint40},
		{BoardVsSurvival, RuleSurvival},
		{BoardVsScore120, RuleScore120},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		lb.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/stack-ws/leaderboard?board="+c.board, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s code = %d", c.board, rec.Code)
		}
		store.mu.Lock()
		got := store.lastRule
		store.mu.Unlock()
		if got != c.rule {
			t.Fatalf("%s → 규칙 %q, want %q", c.board, got, c.rule)
		}
		var out struct {
			Board string      `json:"board"`
			Rows  []VersusRow `json:"rows"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if out.Board != c.board || len(out.Rows) != 1 || out.Rows[0].Rank != 1 {
			t.Fatalf("%s out = %+v", c.board, out)
		}
	}
	before := store.count()
	for _, board := range []string{"vs_score60", "vs_", "survival", "score120", "VS_SCORE120", "VS_SURVIVAL", ""} {
		rec := httptest.NewRecorder()
		lb.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/stack-ws/leaderboard?board="+board, nil))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("알 수 없는 보드 %q = %d", board, rec.Code)
		}
	}
	if store.count() != before {
		t.Fatal("알 수 없는 보드로 저장소를 읽었다")
	}
	if rule, ok := versusBoardRule(BoardSoloAll); ok || rule != "" {
		t.Fatalf("혼자하기 보드가 대전 규칙으로 매핑됐다 (%q)", rule)
	}
}

func TestAchievedAtKST(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"2026-10-11 23:59:59", "2026-10-11T23:59:59+09:00"},
		{"2026-10-12 00:00:00", "2026-10-12T00:00:00+09:00"},
		{"", ""},
		{"0000-00-00 00:00:00", ""},
		{"2026-10-11T10:00:00Z", ""},
		{"엉터리", ""},
	}
	for _, c := range cases {
		if got := AchievedAtKST(c.raw); got != c.want {
			t.Fatalf("AchievedAtKST(%q) = %q, want %q", c.raw, got, c.want)
		}
	}
}

func TestAchievedWallKSTConvertsFromAnyZone(t *testing.T) {
	cases := []struct {
		at   time.Time
		want string
	}{
		// UTC 14:59:59 = KST 23:59:59 (같은 날), UTC 15:00 = KST 다음 날 0시.
		{time.Date(2026, 10, 11, 14, 59, 59, 0, time.UTC), "2026-10-11 23:59:59"},
		{time.Date(2026, 10, 11, 15, 0, 0, 0, time.UTC), "2026-10-12 00:00:00"},
		{time.Date(2026, 10, 11, 9, 30, 0, 0, time.FixedZone("PDT", -7*60*60)), "2026-10-12 01:30:00"},
		{time.Date(2026, 10, 12, 0, 0, 0, 0, kst), "2026-10-12 00:00:00"},
	}
	for _, c := range cases {
		wall := achievedWallKST(c.at)
		if wall != c.want {
			t.Fatalf("achievedWallKST(%v) = %q, want %q", c.at, wall, c.want)
		}
		// 저장한 벽시계를 다시 읽으면 같은 순간이어야 한다.
		back, err := time.Parse(time.RFC3339, AchievedAtKST(wall))
		if err != nil || !back.Equal(c.at) {
			t.Fatalf("왕복 %v → %q → %v (%v)", c.at, wall, back, err)
		}
	}
}

func TestLobbyCountsRandomOnly(t *testing.T) {
	s := newTestServer(newFakeStore(), testTiming())
	c1, c2 := newTestClient(s, "p1"), newTestClient(s, "p2")
	sendMsg(t, s, c1, "join_matching_queue", map[string]interface{}{"mode": "random", "rule": "sprint40"})
	sendMsg(t, s, c2, "join_matching_queue", map[string]interface{}{"mode": "favorite", "rule": "attack", "invite": "abcd1234"})
	rec := httptest.NewRecorder()
	s.HandleLobby(rec, httptest.NewRequest(http.MethodGet, "/stack-ws/lobby", nil))
	if rec.Header().Get("Cache-Control") != "public, max-age=60" {
		t.Fatalf("cache = %q", rec.Header().Get("Cache-Control"))
	}
	var out struct {
		Waiting map[string]int `json:"waiting"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Waiting["sprint40"] != 1 || out.Waiting["attack"] != 0 {
		t.Fatalf("waiting = %v", out.Waiting)
	}
}
