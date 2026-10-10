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
}

func (f *fakeBoardStore) SoloTop(_ time.Time, _ int) ([]SoloRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return []SoloRow{{Nickname: "첫째", Score: 70485}, {Nickname: "둘째", Score: 100}}, nil
}

func (f *fakeBoardStore) VersusTop(_ string, _ int) ([]VersusRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
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
	if rec := get("vs_score120"); rec.Code != http.StatusBadRequest {
		t.Fatalf("알 수 없는 보드 = %d", rec.Code)
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
