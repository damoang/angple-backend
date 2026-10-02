package v2

import (
	"testing"
	"time"

	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
)

// ⛔ 이 파일이 지키는 계약 (럭키 포인트 라이브 판정):
//
//   - 마스터 스위치가 꺼지면 아무것도 안 한다(C6). 게시판이 꺼지면 시간대와 무관하게 안 한다.
//   - 댓글은 include_comments=true 일 때만(C1).
//   - 시간대 단계 시각은 키·KST 날짜로 결정적이고, [start,end) 안에 통째로, 서로 겹치지 않는다(C8).
//   - 시간대 안이면 그 단계의 확률·금액, 밖이면 게시판 값(C9).
//
// 주사위·시각·저장소는 모두 주입해 결정적으로 돈다.

type fakeLuckyStore struct {
	cfg        *v2repo.LuckyConfig
	boardOdds  int
	boardCOdds int // 게시판 comment_odds
	boardPts   int
	boardPrize []v2repo.LuckyPrize // 게시판 상품 표(앙복타임)
	boardCalls int
	grants     []fakeGrantCall
	outcome    v2repo.GrantOutcome
}

type fakeGrantCall struct {
	mbID, table, id, kind string
	amount                int
	opt                   v2repo.GrantOptions
}

func (f *fakeLuckyStore) GetLuckyConfig() (*v2repo.LuckyConfig, error) { return f.cfg, nil }

func (f *fakeLuckyStore) GetBoardLucky(string) v2repo.BoardLuckyOdds {
	f.boardCalls++
	return v2repo.BoardLuckyOdds{Odds: f.boardOdds, CommentOdds: f.boardCOdds, Points: f.boardPts, Prizes: f.boardPrize}
}

func (f *fakeLuckyStore) GrantWithOptions(mbID, table, id, kind string, amount int, opt v2repo.GrantOptions) (v2repo.GrantOutcome, error) {
	f.grants = append(f.grants, fakeGrantCall{mbID, table, id, kind, amount, opt})
	if f.outcome != "" {
		return f.outcome, nil
	}
	return v2repo.GrantOutcomeGranted, nil
}

// fakeRoller 는 항상 당첨(또는 항상 꽝)이고, 받은 확률·금액을 기록한다.
type fakeRoller struct {
	win    bool
	calls  int
	odds   int
	points int
}

func (r *fakeRoller) RollLucky(dice, maxAmount int) (bool, int) {
	r.calls++
	r.odds, r.points = dice, maxAmount
	if !r.win {
		return false, 0
	}
	return true, maxAmount
}

func enabledCfg() *v2repo.LuckyConfig {
	c := v2repo.DefaultLuckyConfig()
	c.Enabled = true
	return c
}

func newTestLive(store *fakeLuckyStore, roller *fakeRoller, key []byte, now time.Time) *LuckyLive {
	l := NewLuckyLive(store, roller, key)
	l.now = func() time.Time { return now }
	return l
}

var testNow = time.Date(2026, 10, 1, 12, 0, 0, 0, luckyKST)

// TestLuckyLive_MasterOff — C6: 마스터 스위치 off 면 게시판 조회·주사위·지급 모두 없음.
func TestLuckyLive_MasterOff(t *testing.T) {
	store := &fakeLuckyStore{cfg: v2repo.DefaultLuckyConfig(), boardOdds: 1, boardPts: 100}
	roller := &fakeRoller{win: true}
	res := newTestLive(store, roller, []byte("k"), testNow).Process("member_a", "free", 1, false, 0)
	if res.Rolled || roller.calls != 0 || len(store.grants) != 0 || store.boardCalls != 0 {
		t.Fatalf("꺼져 있으면 아무것도 안 해야 한다: res=%+v roller=%d grants=%d board=%d",
			res, roller.calls, len(store.grants), store.boardCalls)
	}
}

// TestLuckyLive_CommentsExcludedByDefault — C1: include_comments=false 면 댓글은 지급 0.
func TestLuckyLive_CommentsExcludedByDefault(t *testing.T) {
	store := &fakeLuckyStore{cfg: enabledCfg(), boardOdds: 1, boardCOdds: 1, boardPts: 100}
	roller := &fakeRoller{win: true}
	l := newTestLive(store, roller, []byte("k"), testNow)

	res := l.Process("member_a", "free", 1, true, 50)
	if res.Rolled || roller.calls != 0 || len(store.grants) != 0 {
		t.Fatalf("댓글은 기본 제외: res=%+v grants=%d", res, len(store.grants))
	}

	// 같은 조건에서 글은 지급된다(대조).
	res = l.Process("member_a", "free", 2, false, 0)
	if !res.Won || len(store.grants) != 1 {
		t.Fatalf("글은 지급돼야 한다: res=%+v grants=%d", res, len(store.grants))
	}

	// include_comments=true 면 댓글도 진행.
	store.cfg.IncludeComments = true
	res = l.Process("member_a", "free", 3, true, 50)
	if !res.Won || len(store.grants) != 2 {
		t.Fatalf("include_comments=true 면 댓글도 진행: res=%+v grants=%d", res, len(store.grants))
	}
}

// TestLuckyLive_BoardOffIgnoresWindows 는 게시판 lucky 가 꺼져 있으면 시간대 안이어도 발동하지 않는지 본다.
func TestLuckyLive_BoardOffIgnoresWindows(t *testing.T) {
	cfg := enabledCfg()
	cfg.WindowStartHour, cfg.WindowEndHour = 0, 24
	cfg.Windows = []v2repo.LuckyWindow{{Name: "온종일", Minutes: 24 * 60, Odds: 1, Points: 9}}
	store := &fakeLuckyStore{cfg: cfg} // 게시판 0,0
	roller := &fakeRoller{win: true}
	res := newTestLive(store, roller, []byte("k"), testNow).Process("member_a", "promotion", 1, false, 0)
	if res.Rolled || roller.calls != 0 || len(store.grants) != 0 {
		t.Fatalf("게시판이 꺼져 있으면 시간대와 무관하게 미발동: res=%+v", res)
	}
}

// TestLuckyLive_PassesCapsAndTier 는 설정의 상한·단계 이름·글 키가 지급에 그대로 넘어가는지(C5 연결), 상한 결과를 돌려주는지 본다.
func TestLuckyLive_PassesCapsAndTier(t *testing.T) {
	store := &fakeLuckyStore{cfg: enabledCfg(), boardOdds: 13, boardPts: 90, outcome: v2repo.GrantOutcomeCappedMember}
	roller := &fakeRoller{win: true}
	res := newTestLive(store, roller, nil, testNow).Process("member_a", "free", 42, false, 0)
	if len(store.grants) != 1 {
		t.Fatalf("당첨이면 지급 시도 1회, got %d", len(store.grants))
	}
	g := store.grants[0]
	if g.opt.MemberDailyCap != 1 || g.opt.DailyCap != 10 {
		t.Errorf("기본 상한 1/10 이 넘어가야 한다, got %d/%d", g.opt.MemberDailyCap, g.opt.DailyCap)
	}
	if g.opt.TierName != LuckyBaseTierName || res.Tier != LuckyBaseTierName {
		t.Errorf("평소 단계 이름은 기본 base_name(앙팡), got %q", g.opt.TierName)
	}
	if g.table != "free" || g.id != "42" || g.kind != "point" || g.amount != 90 {
		t.Errorf("지급 키·금액: %+v", g)
	}
	if !g.opt.Now.Equal(testNow) {
		t.Errorf("판정 시각이 그대로 넘어가야 한다(경계 일관성), got %s", g.opt.Now)
	}
	if res.Outcome != v2repo.GrantOutcomeCappedMember || res.Err != nil {
		t.Errorf("상한 결과를 돌려줘야 한다: %+v", res)
	}
}

// TestLuckyLive_LostRollDoesNotGrant 는 꽝이면 지급을 부르지 않는지 본다(상한 확인은 당첨일 때만).
func TestLuckyLive_LostRollDoesNotGrant(t *testing.T) {
	store := &fakeLuckyStore{cfg: enabledCfg(), boardOdds: 13, boardPts: 90}
	roller := &fakeRoller{win: false}
	res := newTestLive(store, roller, nil, testNow).Process("member_a", "free", 1, false, 0)
	if !res.Rolled || res.Won || len(store.grants) != 0 {
		t.Fatalf("꽝이면 지급 없음: res=%+v grants=%d", res, len(store.grants))
	}
}

func angpangCfg() *v2repo.LuckyConfig {
	c := enabledCfg()
	c.Windows = []v2repo.LuckyWindow{
		{Name: "앙팡타임", Minutes: 45, Odds: 7, Points: 50},
		{Name: "앙팡팡타임", Minutes: 20, Odds: 3, Points: 70},
	}
	return c
}

func kstDay(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, luckyKST)
}

// TestLuckyWindows_Deterministic — C8: 같은 키·날짜면 몇 번을 계산해도(파드·재시작 무관) 같은 구간.
func TestLuckyWindows_Deterministic(t *testing.T) {
	key := DeriveLuckyWindowKey("test-secret")
	cfg := angpangCfg()
	day := kstDay(2026, 10, 1)
	a := luckyWindowsForDay(key, day, cfg)
	b := luckyWindowsForDay(DeriveLuckyWindowKey("test-secret"), day, angpangCfg())
	if len(a) != 2 || len(b) != 2 {
		t.Fatalf("단계 2개가 열려야 한다, got %d/%d", len(a), len(b))
	}
	for i := range a {
		if !a[i].start.Equal(b[i].start) || !a[i].end.Equal(b[i].end) || a[i].name != b[i].name {
			t.Errorf("결정적이어야 한다: %d번 단계가 다르다", i)
		}
	}
	// 같은 순간을 UTC 로 줘도(날짜 문자열은 KST 기준) 같아야 한다.
	c := luckyWindowsForDay(key, day.UTC(), cfg)
	for i := range a {
		if !a[i].start.Equal(c[i].start) {
			t.Errorf("입력 시간대와 무관해야 한다: %d번", i)
		}
	}
	// 다른 키면 (거의 항상) 다른 시각 — 키가 실제로 쓰이는지 확인.
	d := luckyWindowsForDay(DeriveLuckyWindowKey("other-secret"), day, cfg)
	if a[0].start.Equal(d[0].start) && a[1].start.Equal(d[1].start) {
		t.Error("키가 바뀌었는데 두 단계 시각이 모두 같다 — 키가 반영되지 않는 것 같다")
	}
}

// TestLuckyWindows_RangeAndNoOverlap — C8: 365일 동안 매일 [09:00, 23:00) 안에 통째로, 서로 겹치지 않음. 날짜별로 퍼짐.
func TestLuckyWindows_RangeAndNoOverlap(t *testing.T) {
	key := DeriveLuckyWindowKey("test-secret")
	cfg := angpangCfg()
	distinct := map[int]bool{}
	for i := 0; i < 365; i++ {
		day := kstDay(2026, 1, 1).AddDate(0, 0, i)
		ws := luckyWindowsForDay(key, day, cfg)
		if len(ws) != 2 {
			t.Fatalf("%s: 단계 2개가 열려야 한다, got %d", day.Format("2006-01-02"), len(ws))
		}
		lo := day.Add(9 * time.Hour)
		hi := day.Add(23 * time.Hour)
		for _, w := range ws {
			if w.start.Before(lo) || w.end.After(hi) {
				t.Errorf("%s: %s 범위 밖", day.Format("2006-01-02"), w.name)
			}
			if w.end.Sub(w.start) != time.Duration(map[string]int{"앙팡타임": 45, "앙팡팡타임": 20}[w.name])*time.Minute {
				t.Errorf("%s: %s 길이가 다르다", day.Format("2006-01-02"), w.name)
			}
		}
		if ws[0].start.Before(ws[1].end) && ws[1].start.Before(ws[0].end) {
			t.Errorf("%s: 두 단계가 겹친다", day.Format("2006-01-02"))
		}
		distinct[int(ws[0].start.Sub(day).Minutes())] = true
	}
	// 가능한 시작 분은 796개. 365일이면 서로 다른 값이 충분히 많아야 한다(쏠림 감지용 느슨한 하한).
	if len(distinct) < 200 {
		t.Errorf("시작 시각이 날짜별로 고르게 퍼지지 않는다: 서로 다른 값 %d개", len(distinct))
	}
}

// TestLuckyWindows_TightRangeNeverOverlaps 는 자리가 모자라면 겹쳐 열지 않고 뒤 단계를 건너뛰는지 본다.
func TestLuckyWindows_TightRangeNeverOverlaps(t *testing.T) {
	key := DeriveLuckyWindowKey("test-secret")
	cfg := enabledCfg()
	cfg.WindowStartHour, cfg.WindowEndHour = 9, 11
	cfg.Windows = []v2repo.LuckyWindow{
		{Name: "꽉참", Minutes: 120, Odds: 2, Points: 10},
		{Name: "자리없음", Minutes: 20, Odds: 2, Points: 10},
	}
	for i := 0; i < 50; i++ {
		ws := luckyWindowsForDay(key, kstDay(2026, 10, 1).AddDate(0, 0, i), cfg)
		if len(ws) != 1 || ws[0].name != "꽉참" {
			t.Fatalf("범위를 꽉 채운 첫 단계만 열려야 한다, got %d", len(ws))
		}
	}
}

// TestLuckyWindows_OffWithoutSecretOrConfig — C8: 키가 없거나, 단계가 없거나, 시 범위가 잘못되면 시간대 없음.
func TestLuckyWindows_OffWithoutSecretOrConfig(t *testing.T) {
	if DeriveLuckyWindowKey("") != nil {
		t.Error("시크릿이 비면 키도 nil 이어야 한다")
	}
	day := kstDay(2026, 10, 1)
	if ws := luckyWindowsForDay(nil, day, angpangCfg()); ws != nil {
		t.Error("키가 없으면 단계 없음")
	}
	key := DeriveLuckyWindowKey("test-secret")
	if ws := luckyWindowsForDay(key, day, enabledCfg()); ws != nil {
		t.Error("windows 가 비면 단계 없음(앙복만)")
	}
	bad := angpangCfg()
	bad.WindowStartHour, bad.WindowEndHour = 23, 9
	if ws := luckyWindowsForDay(key, day, bad); ws != nil {
		t.Error("시작 >= 끝이면 단계 없음")
	}

	// 키가 없으면 단계 설정이 있어도 언제나 앙복타임(게시판 값)으로 판정.
	store := &fakeLuckyStore{cfg: angpangCfg(), boardOdds: 13, boardPts: 90}
	roller := &fakeRoller{win: true}
	l := newTestLive(store, roller, nil, testNow)
	for m := 0; m < 24*60; m += 7 {
		l.now = func() time.Time { return day.Add(time.Duration(m) * time.Minute) }
		res := l.Process("member_a", "free", m, false, 0)
		if res.Tier != LuckyBaseTierName || roller.odds != 13 || roller.points != 90 {
			t.Fatalf("키 없음 → 항상 앙복타임: %+v", res)
		}
	}
}

// TestLuckyLive_WindowPicksTierOddsPoints — C9: 시간대 안이면 그 단계의 확률·금액·이름, 바로 밖이면 게시판 값.
func TestLuckyLive_WindowPicksTierOddsPoints(t *testing.T) {
	key := DeriveLuckyWindowKey("test-secret")
	cfg := angpangCfg()
	day := kstDay(2026, 10, 1)
	ws := luckyWindowsForDay(key, day, cfg)
	if len(ws) != 2 {
		t.Fatalf("단계 2개가 열려야 한다, got %d", len(ws))
	}

	store := &fakeLuckyStore{cfg: cfg, boardOdds: 13, boardPts: 90}
	roller := &fakeRoller{win: true}
	l := newTestLive(store, roller, key, testNow)

	check := func(at time.Time, wantTier string, wantOdds, wantPts int) {
		t.Helper()
		l.now = func() time.Time { return at }
		res := l.Process("member_a", "free", 1, false, 0)
		if res.Tier != wantTier || roller.odds != wantOdds || roller.points != wantPts {
			t.Errorf("단계 판정: got tier=%q odds=%d pts=%d, want %q %d %d",
				res.Tier, roller.odds, roller.points, wantTier, wantOdds, wantPts)
		}
		last := store.grants[len(store.grants)-1]
		if last.opt.TierName != wantTier {
			t.Errorf("지급 단계 이름: got %q want %q", last.opt.TierName, wantTier)
		}
	}

	for _, w := range ws {
		// 시작 순간(포함), 끝 1초 전(포함) → 그 단계
		check(w.start, w.name, w.odds, w.points)
		check(w.end.Add(-time.Second), w.name, w.odds, w.points)
		// UTC 로 들어와도 같은 판정
		check(w.start.UTC(), w.name, w.odds, w.points)
	}
	// 두 단계 어디에도 안 걸리는 시각을 찾아 앙복타임인지 본다(끝 순간은 제외 구간).
	for _, w := range ws {
		at := w.end
		inOther := false
		for _, o := range ws {
			if !at.Before(o.start) && at.Before(o.end) {
				inOther = true
			}
		}
		if !inOther {
			check(at, LuckyBaseTierName, 13, 90)
		}
	}
	// 범위 밖(새벽)은 항상 앙복타임.
	check(day.Add(3*time.Hour), LuckyBaseTierName, 13, 90)
	check(day.Add(23*time.Hour+30*time.Minute), LuckyBaseTierName, 13, 90)
}

// ── 댓글 발동(L8~L12) ──────────────────────────────────────────────────────────

func commentCfg() *v2repo.LuckyConfig {
	c := enabledCfg()
	c.IncludeComments = true
	c.DailyCapPost, c.DailyCapComment = 3, 4
	c.ExpireDays = 7
	return c
}

// TestLuckyCommentChars — L10/C12: HTML 태그·이모티콘 숏코드·앞뒤 공백을 빼고 rune 수를 센다.
func TestLuckyCommentChars(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"   ", 0},
		{"가나다라마바사아자차", 10},
		{"  가나다라마바사아자차  ", 10},
		{"<p>가나다</p>", 3},
		{"<p><b>가나</b> 다</p>", 4}, // 안쪽 공백은 센다
		{"{emo:smile.gif}", 0},    // 이모티콘만
		{"{이모티콘:하트}{emo:a.png}  ", 0},
		{"<p>{emo:x.gif}</p>", 0},
		{"좋아요{emo:x.gif}", 3},
		{"abc&nbsp;", 3}, // 엔티티 공백은 앞뒤에서 지워진다
		{"&lt;3", 2},
		{"{emoji:x}", 9}, // 다른 중괄호는 숏코드가 아니다
		{"<img src=\"a.png\">", 0},
	}
	for _, c := range cases {
		if got := LuckyCommentChars(c.in); got != c.want {
			t.Errorf("LuckyCommentChars(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

// TestLuckyLive_CommentMinChars — C12: 정리 길이가 min_comment_chars 미만이면 댓글은 주사위까지 가지 않는다. 글은 길이 무관.
func TestLuckyLive_CommentMinChars(t *testing.T) {
	cfg := commentCfg()
	cfg.MinCommentChars = 6
	store := &fakeLuckyStore{cfg: cfg, boardOdds: 1, boardCOdds: 1, boardPts: 60}
	roller := &fakeRoller{win: true}
	l := newTestLive(store, roller, nil, testNow)

	if res := l.Process("member_a", "free", 1, true, 5); res.Rolled || len(store.grants) != 0 {
		t.Fatalf("5자 댓글은 제외(하한 6): res=%+v", res)
	}
	if res := l.Process("member_a", "free", 2, true, LuckyCommentChars("{emo:x.gif}{emo:y.gif}")); res.Rolled || len(store.grants) != 0 {
		t.Fatalf("이모티콘만 댓글은 제외: res=%+v", res)
	}
	if res := l.Process("member_a", "free", 3, true, 6); !res.Won || len(store.grants) != 1 {
		t.Fatalf("하한과 같은 길이면 진행: res=%+v grants=%d", res, len(store.grants))
	}
	if res := l.Process("member_a", "free", 4, false, 0); !res.Won || len(store.grants) != 2 {
		t.Fatalf("글은 길이 제한 없음: res=%+v grants=%d", res, len(store.grants))
	}

	// 설정에 키가 없으면 기본 하한(10) — 9자는 제외, 10자는 진행.
	def := enabledCfg()
	def.IncludeComments = true
	store2 := &fakeLuckyStore{cfg: def, boardOdds: 1, boardCOdds: 1, boardPts: 60}
	l2 := newTestLive(store2, &fakeRoller{win: true}, nil, testNow)
	if res := l2.Process("member_a", "free", 1, true, 9); res.Rolled {
		t.Fatalf("기본 하한 미만은 제외: res=%+v", res)
	}
	if res := l2.Process("member_a", "free", 2, true, 10); !res.Won {
		t.Fatalf("기본 하한 이상은 진행: res=%+v", res)
	}
}

// TestLuckyLive_CommentOddsMissing — C13/L9: 게시판 comment_odds 가 없으면 댓글 미발동(글은 그대로).
func TestLuckyLive_CommentOddsMissing(t *testing.T) {
	store := &fakeLuckyStore{cfg: commentCfg(), boardOdds: 1, boardPts: 60} // comment_odds 없음
	roller := &fakeRoller{win: true}
	l := newTestLive(store, roller, nil, testNow)

	if res := l.Process("member_a", "free", 1, true, 50); res.Rolled || roller.calls != 0 || len(store.grants) != 0 {
		t.Fatalf("comment_odds 없으면 댓글 미발동: res=%+v", res)
	}
	if res := l.Process("member_a", "free", 2, false, 0); !res.Won || len(store.grants) != 1 {
		t.Fatalf("글은 그대로 발동: res=%+v", res)
	}
}

// TestLuckyLive_WindowWithoutCommentOdds — C13/L9: 시간대 단계에 comment_odds 가 없으면 그 단계 안에서 댓글은 미발동,
// comment_odds 가 있는 단계는 그 확률을 쓰고 금액은 단계 points 를 같이 쓴다.
func TestLuckyLive_WindowWithoutCommentOdds(t *testing.T) {
	key := DeriveLuckyWindowKey("test-secret")
	cfg := commentCfg()
	cfg.Windows = []v2repo.LuckyWindow{
		{Name: "단계A", Minutes: 45, Odds: 7, Points: 50},                  // 댓글 확률 없음
		{Name: "단계B", Minutes: 20, Odds: 3, CommentOdds: 11, Points: 70}, // 댓글 확률 있음
	}
	day := kstDay(2026, 10, 1)
	ws := luckyWindowsForDay(key, day, cfg)
	if len(ws) != 2 {
		t.Fatalf("단계 2개가 열려야 한다, got %d", len(ws))
	}
	store := &fakeLuckyStore{cfg: cfg, boardOdds: 13, boardCOdds: 17, boardPts: 90}
	roller := &fakeRoller{win: true}
	l := newTestLive(store, roller, key, testNow)

	for _, w := range ws {
		l.now = func() time.Time { return w.start }
		roller.calls = 0
		before := len(store.grants)
		res := l.Process("member_a", "free", 1, true, 50)
		switch w.name {
		case "단계A":
			if res.Rolled || roller.calls != 0 || len(store.grants) != before {
				t.Errorf("단계A 안에서 댓글은 미발동: res=%+v", res)
			}
			// 같은 단계에서 글은 발동(대조).
			if res := l.Process("member_a", "free", 2, false, 0); !res.Won || roller.odds != 7 {
				t.Errorf("단계A 안에서 글은 단계 확률로 발동: res=%+v odds=%d", res, roller.odds)
			}
		case "단계B":
			if !res.Won || roller.odds != 11 || roller.points != 70 || res.Tier != "단계B" {
				t.Errorf("단계B 댓글은 comment_odds·단계 points: res=%+v odds=%d pts=%d", res, roller.odds, roller.points)
			}
		}
	}

	// 시간대 밖(새벽)은 게시판 comment_odds·points.
	l.now = func() time.Time { return day.Add(3 * time.Hour) }
	if res := l.Process("member_a", "free", 3, true, 50); !res.Won || roller.odds != 17 || roller.points != 90 || res.Tier != LuckyBaseTierName {
		t.Errorf("앙복타임 댓글은 게시판 comment_odds: res=%+v odds=%d", res, roller.odds)
	}
}

// TestLuckyLive_KindCapsAndExpiryPassed — L8/L13: 글은 kind=point·글 상한, 댓글은 kind=cpoint·댓글 상한.
// 회원 상한(합산)과 만료 일수는 둘 다 같은 값이 넘어간다.
func TestLuckyLive_KindCapsAndExpiryPassed(t *testing.T) {
	store := &fakeLuckyStore{cfg: commentCfg(), boardOdds: 1, boardCOdds: 1, boardPts: 60}
	l := newTestLive(store, &fakeRoller{win: true}, nil, testNow)

	l.Process("member_a", "free", 10, false, 0)
	l.Process("member_a", "free", 11, true, 50)
	if len(store.grants) != 2 {
		t.Fatalf("지급 시도 2회, got %d", len(store.grants))
	}
	post, cmt := store.grants[0], store.grants[1]
	if post.kind != v2repo.LuckyKindPost || post.opt.DailyCap != 3 {
		t.Errorf("글: kind=point·글 상한, got kind=%s cap=%d", post.kind, post.opt.DailyCap)
	}
	if cmt.kind != v2repo.LuckyKindComment || cmt.opt.DailyCap != 4 || cmt.id != "11" {
		t.Errorf("댓글: kind=cpoint·댓글 상한·댓글 wr_id, got kind=%s cap=%d id=%s", cmt.kind, cmt.opt.DailyCap, cmt.id)
	}
	for _, g := range store.grants {
		if g.opt.MemberDailyCap != 1 {
			t.Errorf("회원 상한(합산)은 종류 무관 같은 값, got %d", g.opt.MemberDailyCap)
		}
		if g.opt.ExpireDays != 7 {
			t.Errorf("만료 일수가 넘어가야 한다, got %d", g.opt.ExpireDays)
		}
	}

	// 설정에 키가 없으면 만료 기본(365일)이 넘어간다.
	store2 := &fakeLuckyStore{cfg: enabledCfg(), boardOdds: 1, boardPts: 60}
	newTestLive(store2, &fakeRoller{win: true}, nil, testNow).Process("member_a", "free", 1, false, 0)
	if len(store2.grants) != 1 || store2.grants[0].opt.ExpireDays != 365 {
		t.Errorf("만료 기본 365일, got %+v", store2.grants)
	}
}
