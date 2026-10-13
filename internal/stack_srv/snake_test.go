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

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// botVector 는 점수가 나는 공용 벡터 하나(봇 0, seed 42)다.
func botVector(t *testing.T) snakeVector {
	t.Helper()
	for _, v := range loadSnakeVectors(t) {
		if v.Seed == 42 && v.Expect.Error == "" && v.Expect.Score > 0 {
			return v
		}
	}
	t.Fatal("seed 42 벡터가 없다")
	return snakeVector{}
}

func TestSnakeClaimCheck(t *testing.T) {
	v := botVector(t)
	enough := v.Expect.SimMs + 1000
	if c := SnakeClaimCheck(v.Seed, v.Inputs, float64(v.Expect.Score), enough); !c.OK() || c.Replay.Ticks != v.Expect.Ticks {
		t.Fatalf("정상 판 = %v", c.Reasons)
	}
	cases := []struct {
		name    string
		inputs  [][]float64
		score   float64
		elapsed int64
		want    string
	}{
		{"점수 부풀림", v.Inputs, float64(v.Expect.Score + 1), enough, snakeFlagScore},
		{"빨리 감기", v.Inputs, float64(v.Expect.Score), 1000, snakeFlagTooFast},
		{"반대 방향 입력", [][]float64{{0, snakeLeft}}, 0, enough, snakeErrDir},
		{"틱 순서", [][]float64{{2, 0}, {1, 1}}, 0, enough, snakeErrOrder},
		{"끝난 뒤 입력", [][]float64{{500, 0}}, 0, enough, snakeErrAfterEnd},
		{"입력 없음(null)", nil, 0, enough, snakeErrShape},
		{"소수 점수", v.Inputs, 1.5, enough, snakeFlagBadScore},
		{"음수 점수", v.Inputs, -1, enough, snakeFlagBadScore},
		{"너무 큰 점수", v.Inputs, 1e12, enough, snakeFlagBadScore},
	}
	for _, c := range cases {
		r := SnakeClaimCheck(v.Seed, c.inputs, c.score, c.elapsed)
		found := false
		for _, code := range r.Reasons {
			found = found || code == c.want
		}
		if r.OK() || !found {
			t.Fatalf("%s: reasons = %v, want %s", c.name, r.Reasons, c.want)
		}
	}
	// 시간 여유 = 실경과 × 1.05 + 2초.
	sim := v.Expect.SimMs
	edge := (sim - SnakeTimeSlackMs) * 100 / (100 + SnakeTimeSlackPercent)
	for edge+edge*SnakeTimeSlackPercent/100+SnakeTimeSlackMs < sim {
		edge++
	}
	if c := SnakeClaimCheck(v.Seed, v.Inputs, float64(v.Expect.Score), edge); !c.OK() {
		t.Fatalf("여유 경계 %dms = %v", edge, c.Reasons)
	}
	if c := SnakeClaimCheck(v.Seed, v.Inputs, float64(v.Expect.Score), edge-50); c.OK() {
		t.Fatalf("여유 밖 %dms 가 통과했다", edge-50)
	}
	if storedSnakeScore(1e12) != snakeStoredScoreMax || storedSnakeScore(-3) != 0 || storedSnakeScore(7) != 7 {
		t.Fatal("저장 점수 자르기")
	}
}

// fakeSnakeStore 는 SnakeStore 의 메모리 가짜다.
type fakeSnakeStore struct {
	// mu 는 records 를 지킨다.
	mu sync.Mutex
	// records 는 저장된 판이다.
	records []SnakeRecord
}

func (f *fakeSnakeStore) SaveSnakeRun(rec SnakeRecord) (SoloStanding, error) {
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

func newTestSnake(store SnakeStore) (*SnakeService, *testClock) {
	clock := &testClock{t: time.Date(2026, 10, 13, 12, 0, 0, 0, kst)}
	verify := func(token string) (string, string, error) {
		if strings.HasPrefix(token, "member-") {
			return strings.TrimPrefix(token, "member-"), "닉", nil
		}
		return "", "", errors.New("bad token")
	}
	svc := NewSnakeService(store, verify)
	svc.issuer.now = clock.now
	return svc, clock
}

// issueRun 은 시드를 정해 판을 하나 심는다(발급기 내부 맵에 직접).
func issueRun(svc *SnakeService, mbID string, seed uint32, at time.Time) string {
	svc.issuer.mu.Lock()
	defer svc.issuer.mu.Unlock()
	id := "r_test_" + mbID
	svc.issuer.runs[id] = &soloRun{id: id, mbID: mbID, seed: seed, startedAt: at}
	svc.issuer.byMember[mbID] = id
	return id
}

func snakePost(svc *SnakeService, path, token string, body []byte) (int, map[string]interface{}) {
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
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

func mustJSON(t *testing.T, v interface{}) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestSnakeStartAndFinishFlow(t *testing.T) {
	store := &fakeSnakeStore{}
	svc, clock := newTestSnake(store)

	if code, _ := snakePost(svc, "/stack-ws/snake/start", "", nil); code != http.StatusUnauthorized {
		t.Fatalf("비로그인 start = %d", code)
	}
	code, started := snakePost(svc, "/stack-ws/snake/start", "member-p1", nil)
	if code != http.StatusOK || started["runId"] == nil || started["seed"] == nil {
		t.Fatalf("start = %d %v", code, started)
	}
	// 받은 시드로 입력 없이 끝까지 — 서버 재연과 같은 점수면 통과.
	seed := uint32(started["seed"].(float64))
	rep := ReplaySnake(seed, [][]float64{})
	clock.advance(time.Duration(rep.SimMs) * time.Millisecond)
	body := map[string]interface{}{"runId": started["runId"], "inputs": [][]float64{}, "score": rep.Score}
	code, res := snakePost(svc, "/stack-ws/snake/finish", "member-p1", mustJSON(t, body))
	if code != http.StatusOK || res["accepted"] != true {
		t.Fatalf("정상 finish = %d %v", code, res)
	}
	// 같은 runId 재제출은 받지 않는다.
	if code, _ := snakePost(svc, "/stack-ws/snake/finish", "member-p1", mustJSON(t, body)); code != http.StatusConflict {
		t.Fatalf("재제출 = %d", code)
	}

	// 점수가 나는 판(공용 벡터) — 정상, 점수 부풀림, 빨리 감기.
	v := botVector(t)
	runs := []struct {
		score   int
		advance time.Duration
		want    bool
		reason  string
	}{
		{v.Expect.Score, time.Duration(v.Expect.SimMs) * time.Millisecond, true, ""},
		{v.Expect.Score + 10, time.Duration(v.Expect.SimMs) * time.Millisecond, false, snakeFlagScore},
		{v.Expect.Score, 5 * time.Second, false, snakeFlagTooFast},
	}
	for i, r := range runs {
		id := issueRun(svc, "p2", v.Seed, clock.now())
		clock.advance(r.advance)
		code, res := snakePost(svc, "/stack-ws/snake/finish", "member-p2",
			mustJSON(t, map[string]interface{}{"runId": id, "inputs": v.Inputs, "score": r.score}))
		if code != http.StatusOK || res["accepted"] != r.want {
			t.Fatalf("#%d finish = %d %v", i, code, res)
		}
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.records) != 4 {
		t.Fatalf("records = %d", len(store.records))
	}
	ok, inflated, fast := store.records[1], store.records[2], store.records[3]
	if ok.Flagged || ok.Score != int64(v.Expect.Score) || ok.Ticks != int64(v.Expect.Ticks) || ok.Seed != 42 || !strings.HasPrefix(ok.InputsJSON, "[[") {
		t.Fatalf("정상 기록 = %+v", ok)
	}
	if !inflated.Flagged || !strings.Contains(inflated.FlagReason, snakeFlagScore) {
		t.Fatalf("부풀린 기록 = %+v", inflated)
	}
	if !fast.Flagged || !strings.Contains(fast.FlagReason, snakeFlagTooFast) || fast.ElapsedMs != 5000 {
		t.Fatalf("빨리 감은 기록 = %+v", fast)
	}
}

func TestSnakeFinishRejectsBadRequests(t *testing.T) {
	svc, clock := newTestSnake(&fakeSnakeStore{})
	id := issueRun(svc, "p1", 1, clock.now())

	// 메서드·인증
	rec := httptest.NewRecorder()
	svc.HandleFinish(rec, httptest.NewRequest(http.MethodGet, "/stack-ws/snake/finish", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET = %d", rec.Code)
	}
	if code, _ := snakePost(svc, "/stack-ws/snake/finish", "nope", []byte(`{}`)); code != http.StatusUnauthorized {
		t.Fatalf("나쁜 토큰 = %d", code)
	}
	// 128KB 넘는 본문
	big := `{"runId":"` + id + `","score":0,"inputs":[` + strings.Repeat(`[1,1],`, 30000) + `[1,1]]}`
	if code, _ := snakePost(svc, "/stack-ws/snake/finish", "member-p1", []byte(big)); code != http.StatusBadRequest {
		t.Fatalf("큰 본문 = %d", code)
	}
	// 입력 길이 상한(본문은 128KB 안)
	many := `{"runId":"` + id + `","score":0,"inputs":[` + strings.Repeat(`[],`, SnakeMaxTicks) + `[]]}`
	if len(many) > snakeBodyMax {
		t.Fatalf("테스트 본문이 너무 크다: %d", len(many))
	}
	if code, _ := snakePost(svc, "/stack-ws/snake/finish", "member-p1", []byte(many)); code != http.StatusBadRequest {
		t.Fatalf("입력 길이 초과 = %d", code)
	}
	// 깨진 JSON
	if code, _ := snakePost(svc, "/stack-ws/snake/finish", "member-p1", []byte(`{"runId":`)); code != http.StatusBadRequest {
		t.Fatalf("깨진 JSON = %d", code)
	}
	// 남의 판
	if code, _ := snakePost(svc, "/stack-ws/snake/finish", "member-p9", mustJSON(t, map[string]interface{}{"runId": id, "inputs": [][]float64{}, "score": 0})); code != http.StatusConflict {
		t.Fatalf("남의 판 = %d", code)
	}
	// 만료된 판
	clock.advance(SoloRunTTL + time.Second)
	if code, _ := snakePost(svc, "/stack-ws/snake/finish", "member-p1", mustJSON(t, map[string]interface{}{"runId": id, "inputs": [][]float64{}, "score": 0})); code != http.StatusConflict {
		t.Fatalf("만료된 판 = %d", code)
	}
}

func TestSnakeStartIsSeparateFromStackSolo(t *testing.T) {
	svc, _ := newTestSnake(&fakeSnakeStore{})
	if code, _ := snakePost(svc, "/stack-ws/snake/start", "member-p1", nil); code != http.StatusOK {
		t.Fatalf("첫 start = %d", code)
	}
	if code, _ := snakePost(svc, "/stack-ws/snake/start", "member-p1", nil); code != http.StatusTooManyRequests {
		t.Fatalf("5초 안 재시작 = %d", code)
	}
	// 쿼리 토큰은 받지 않는다.
	rec := httptest.NewRecorder()
	svc.HandleStart(rec, httptest.NewRequest(http.MethodPost, "/stack-ws/snake/start?token=member-p2", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("쿼리 토큰 = %d", rec.Code)
	}
}

func TestLeaderboardSnakeBoards(t *testing.T) {
	store := &fakeBoardStore{}
	lb := NewLeaderboard(store)
	clock := &testClock{t: time.Date(2026, 10, 13, 12, 0, 0, 0, kst)}
	lb.now = clock.now
	get := func(board string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		lb.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/stack-ws/leaderboard?board="+board, nil))
		return rec
	}
	rec := get(BoardSnakeWeek)
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "public, max-age=60" {
		t.Fatalf("code=%d cache=%q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	var out struct {
		Board     string     `json:"board"`
		WeekStart string     `json:"weekStart"`
		Rows      []SnakeRow `json:"rows"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Board != BoardSnakeWeek || out.WeekStart != "2026-10-12" || len(out.Rows) != 1 || out.Rows[0].Rank != 1 ||
		out.Rows[0].Score != 57 || out.Rows[0].AchievedAt != "2026-10-12T00:00:01+09:00" {
		t.Fatalf("out = %+v", out)
	}
	if body := rec.Body.String(); strings.Contains(body, "mb_id") || strings.Contains(body, "achieved_raw") {
		t.Fatalf("응답 모양이 다르다: %s", body)
	}
	if store.lastSnakeWeek.Format(snakeDayLayout) != "2026-10-12" || store.lastN != LeaderboardSize {
		t.Fatalf("주 = %v n = %d", store.lastSnakeWeek, store.lastN)
	}
	get(BoardSnakeWeek)
	if store.count() != 1 {
		t.Fatal("60초 캐시")
	}
	get(BoardSnakeAll)
	if store.count() != 2 || !store.lastSnakeWeek.IsZero() {
		t.Fatalf("전체 표는 주 없이 읽어야 한다 (%v)", store.lastSnakeWeek)
	}
	// 월요일 0시가 지나면 새 주 표를 읽는다.
	clock.t = time.Date(2026, 10, 19, 0, 0, 1, 0, kst)
	get(BoardSnakeWeek)
	if store.count() != 3 || store.lastSnakeWeek.Format(snakeDayLayout) != "2026-10-19" {
		t.Fatalf("새 주 = %v", store.lastSnakeWeek)
	}
	for _, board := range []string{"snake", "snake_", "SNAKE_WEEK", "snake_month"} {
		if rec := get(board); rec.Code != http.StatusBadRequest {
			t.Fatalf("알 수 없는 보드 %q = %d", board, rec.Code)
		}
	}
}

// newSnakeTestStore 는 sqlite 메모리 DB 에 뱀 게임 표와 회원 표를 만든다.
func newSnakeTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=private"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	stmts := []string{
		`CREATE TABLE g5_member (mb_id TEXT PRIMARY KEY, mb_nick TEXT,
			mb_leave_date TEXT NOT NULL DEFAULT '', mb_intercept_date TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE angple_snake_runs (id INTEGER PRIMARY KEY AUTOINCREMENT, run_id TEXT NOT NULL UNIQUE,
			mb_id TEXT NOT NULL, seed INTEGER NOT NULL, score INTEGER NOT NULL, ticks INTEGER NOT NULL,
			inputs_json TEXT, elapsed_ms INTEGER NOT NULL, flagged INTEGER NOT NULL DEFAULT 0, flag_reason TEXT,
			started_at DATETIME NOT NULL, finished_at DATETIME NOT NULL)`,
		`CREATE TABLE angple_snake_best (mb_id TEXT PRIMARY KEY, score INTEGER NOT NULL, run_id TEXT NOT NULL,
			achieved_at DATETIME NOT NULL)`,
		`CREATE TABLE angple_snake_weekly (week_start DATE NOT NULL, mb_id TEXT NOT NULL, score INTEGER NOT NULL,
			run_id TEXT NOT NULL, achieved_at DATETIME NOT NULL, PRIMARY KEY (week_start, mb_id))`,
		`INSERT INTO g5_member (mb_id, mb_nick, mb_leave_date, mb_intercept_date) VALUES
			('a', '에이', '', ''), ('b', '비', '', ''), ('left', '탈퇴', '20261001', ''), ('blocked', '제한', '', '20261001')`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("%v: %s", err, s)
		}
	}
	return NewStore(db)
}

func TestSnakeStoreSaveAndTop(t *testing.T) {
	s := newSnakeTestStore(t)
	week := time.Date(2026, 10, 12, 0, 0, 0, 0, kst)
	lastWeek := week.AddDate(0, 0, -7)
	n := 0
	save := func(mb string, score int64, flagged bool, ws time.Time) SoloStanding {
		t.Helper()
		n++
		st, err := s.SaveSnakeRun(SnakeRecord{
			RunID: "r_" + mb + "_" + string(rune('a'+n)), MbID: mb, Seed: 7, Score: score, Ticks: 100,
			InputsJSON: "[[0,0]]", ElapsedMs: 15000, Flagged: flagged, FlagReason: map[bool]string{true: snakeFlagScore}[flagged],
			StartedAt: time.Now().Add(-time.Minute), WeekStart: ws,
		})
		if err != nil {
			t.Fatalf("save %s: %v", mb, err)
		}
		return st
	}

	if st := save("a", 30, false, week); st.Best != 30 || st.WeekBest != 30 || st.RankWeek != 1 {
		t.Fatalf("첫 기록 = %+v", st)
	}
	if st := save("a", 10, false, week); st.Best != 30 || st.WeekBest != 30 {
		t.Fatalf("낮은 기록이 최고를 내렸다 = %+v", st)
	}
	if st := save("a", 99, true, week); st.Best != 30 || st.WeekBest != 30 {
		t.Fatalf("flagged 판이 최고를 바꿨다 = %+v", st)
	}
	if st := save("b", 40, false, week); st.Best != 40 || st.RankWeek != 1 {
		t.Fatalf("b = %+v", st)
	}
	if st := save("a", 35, false, week); st.Best != 35 || st.WeekBest != 35 || st.RankWeek != 2 {
		t.Fatalf("a 갱신 = %+v", st)
	}
	save("a", 80, false, lastWeek) // 지난주 기록 — 이번 주 표에는 없고 역대에는 있다
	save("left", 500, false, week)
	save("blocked", 400, false, week)

	var runs, flagged int64
	if err := s.db.Raw("SELECT COUNT(*), COALESCE(SUM(flagged), 0) FROM angple_snake_runs").Row().Scan(&runs, &flagged); err != nil {
		t.Fatal(err)
	}
	if runs != 8 || flagged != 1 {
		t.Fatalf("runs=%d flagged=%d", runs, flagged)
	}

	weekRows, err := s.SnakeTop(week, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(weekRows) != 2 || weekRows[0].Nickname != "비" || weekRows[0].Score != 40 || weekRows[1].Score != 35 {
		t.Fatalf("이번 주 = %+v", weekRows)
	}
	if AchievedAtKST(weekRows[0].AchievedRaw) == "" {
		t.Fatalf("기록 시각을 읽지 못했다: %q", weekRows[0].AchievedRaw)
	}
	allRows, err := s.SnakeTop(time.Time{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(allRows) != 2 || allRows[0].Nickname != "에이" || allRows[0].Score != 80 || allRows[1].Score != 40 {
		t.Fatalf("역대 = %+v", allRows)
	}
	if top1, _ := s.SnakeTop(time.Time{}, 1); len(top1) != 1 {
		t.Fatalf("n 제한 = %d", len(top1))
	}
}
